// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateManagedSoftwareUpdateGroupPlanV1   (POST /plans/group — 201; group-targeted)
//
// Not adopted:
//   pro.CreateManagedSoftwareUpdatePlanV1        (POST /plans — per-device; Terraform targets groups, not individual devices)
//   pro.GetManagedSoftwareUpdatePlanV1 and the statuses/declarations/events GETs (read-only telemetry)
//
// A group plan is a fire-once directive: the spec gives /plans only GET + POST (no PUT,
// PATCH, or DELETE), so it is modelled as an action — no persisted state, no drift, no
// delete. The server expands the target group into one device-plan per member and returns
// the minted planId(s), which the action surfaces in its progress output. Re-invoking
// submits a fresh plan. The endpoint requires the feature toggle ON (503 otherwise) and is
// rate-limited (429); both server errors are surfaced to the caller.

package managed_software_updates

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*PlanAction)(nil)
var _ action.ActionWithConfigure = (*PlanAction)(nil)
var _ action.ActionWithConfigValidators = (*PlanAction)(nil)

// Write-accepted enum subsets (wire-probed 2026-06-13), keyed on the
// PlanConfigurationPost vocabularies — the type this action actually posts.
//
// updateActions and versionTypes are deliberately narrower than the generated
// sets, which each also carry UNKNOWN: versionType=UNKNOWN is wire-rejected
// (400) and updateAction=UNKNOWN is degenerate. The sets are curated; the
// spellings are the SDK's. objectTypes matches its generated set exactly, so it
// calls the helper.
var (
	updateActions = []string{
		pro.PlanConfigurationPostUpdateActionDownloadOnly,
		pro.PlanConfigurationPostUpdateActionDownloadInstall,
		pro.PlanConfigurationPostUpdateActionDownloadInstallAllowDeferral,
		pro.PlanConfigurationPostUpdateActionDownloadInstallRestart,
		pro.PlanConfigurationPostUpdateActionDownloadInstallSchedule,
	}
	versionTypes = []string{
		pro.PlanConfigurationPostVersionTypeLatestMajor,
		pro.PlanConfigurationPostVersionTypeLatestMinor,
		pro.PlanConfigurationPostVersionTypeLatestAny,
		pro.PlanConfigurationPostVersionTypeSpecificVersion,
		pro.PlanConfigurationPostVersionTypeCustomVersion,
	}
	objectTypes = pro.PlanGroupPostObjectTypeValues()
)

// PlanAction submits a Managed Software Updates plan targeting a group.
type PlanAction struct {
	msuAction
}

// PlanActionModel represents the action config schema.
type PlanActionModel struct {
	GroupID                   types.String `tfsdk:"group_id"`
	ObjectType                types.String `tfsdk:"object_type"`
	UpdateAction              types.String `tfsdk:"update_action"`
	VersionType               types.String `tfsdk:"version_type"`
	SpecificVersion           types.String `tfsdk:"specific_version"`
	BuildVersion              types.String `tfsdk:"build_version"`
	ForceInstallLocalDateTime types.String `tfsdk:"force_install_local_date_time"`
	MaxDeferrals              types.Int64  `tfsdk:"max_deferrals"`
}

// NewPlanAction constructs the action.
func NewPlanAction() action.Action {
	return &PlanAction{}
}

func (a *PlanAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_managed_software_update_plan"
}

func (a *PlanAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Submits a Managed Software Updates plan that enforces a target OS version on the members of a smart or static group. " +
			"This is a fire-once directive: each invocation submits a new plan (there is nothing to update, and nothing to destroy). " +
			"The Managed Software Updates feature must be enabled first (see `jamfplatform_pro_managed_software_update`)." +
			planPrivileges,
		Attributes: map[string]actionschema.Attribute{
			"group_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Jamf Pro ID of the target smart or static group. Use the `jamf_pro_id` exported by `jamfplatform_device_group`.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"object_type": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The kind of group being targeted: `COMPUTER_GROUP` or `MOBILE_DEVICE_GROUP`.",
				Validators: []validator.String{
					stringvalidator.OneOf(objectTypes...),
				},
			},
			"update_action": actionschema.StringAttribute{
				Required: true,
				MarkdownDescription: "The install action to take. One of: " +
					"`DOWNLOAD_ONLY` (download to devices only), " +
					"`DOWNLOAD_INSTALL` (download and install), " +
					"`DOWNLOAD_INSTALL_ALLOW_DEFERRAL` (download, install, and allow the user to defer; see `max_deferrals`), " +
					"`DOWNLOAD_INSTALL_RESTART` (download, install, and restart), " +
					"`DOWNLOAD_INSTALL_SCHEDULE` (download and schedule the install; see `force_install_local_date_time`).",
				Validators: []validator.String{
					stringvalidator.OneOf(updateActions...),
				},
			},
			"version_type": actionschema.StringAttribute{
				Required: true,
				MarkdownDescription: "Which OS version to target. One of: " +
					"`LATEST_ANY` (latest version each device is eligible for), " +
					"`LATEST_MAJOR` (latest major version), " +
					"`LATEST_MINOR` (latest minor version), " +
					"`SPECIFIC_VERSION` (a specific OS version; set `specific_version`), " +
					"`CUSTOM_VERSION` (a custom OS version; set `specific_version`).",
				Validators: []validator.String{
					stringvalidator.OneOf(versionTypes...),
				},
			},
			"specific_version": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The OS version to enforce. Required when `version_type` is `SPECIFIC_VERSION` or `CUSTOM_VERSION`; leave unset otherwise.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"build_version": actionschema.StringAttribute{
				Optional: true,
				MarkdownDescription: "A specific OS build to enforce, paired with `specific_version`. " +
					"Only valid when `version_type` is `CUSTOM_VERSION`. Jamf Pro rejects a build version for every other `version_type`, including `SPECIFIC_VERSION`.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"force_install_local_date_time": actionschema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The local date and time by which the update must be installed, in `YYYY-MM-DDThh:mm:ss` form (for example `2026-12-25T21:09:31`). " +
					"Applies when `update_action` is `DOWNLOAD_INSTALL_SCHEDULE`.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						forceInstallLocalDateTimePattern,
						"must be a local date and time in YYYY-MM-DDThh:mm:ss form, for example 2026-12-25T21:09:31",
					),
				},
			},
			"max_deferrals": actionschema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "How many times a user may defer the update, from `0` to `99`. " +
					"Applies when `update_action` is `DOWNLOAD_INSTALL_ALLOW_DEFERRAL`.",
				Validators: []validator.Int64{
					int64validator.Between(0, 99),
				},
			},
		},
	}
}

func (a *PlanAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return []action.ConfigValidator{
		specificVersionRequiredValidator{},
		buildVersionCustomOnlyValidator{},
	}
}

func (a *PlanAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *PlanAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClient(resp) {
		return
	}

	var data PlanActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := buildGroupPlanRequest(data)

	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Submitting Managed Software Updates plan for %s %s", data.ObjectType.ValueString(), data.GroupID.ValueString()),
	})

	result, err := a.client.CreateManagedSoftwareUpdateGroupPlanV1(ctx, request)
	if err != nil {
		resp.Diagnostics.AddError(
			"Managed Software Updates Plan Failed",
			fmt.Sprintf("Unable to submit update plan for %s %s: %s", data.ObjectType.ValueString(), data.GroupID.ValueString(), helpers.APIErrorDetail(err)),
		)
		return
	}

	planIDs := make([]string, 0, len(result.Plans))
	for _, p := range result.Plans {
		if p.PlanID != "" {
			planIDs = append(planIDs, p.PlanID)
		}
	}

	switch len(planIDs) {
	case 0:
		resp.SendProgress(action.InvokeProgressEvent{
			Message: "Update plan accepted; no device plans were created (the target group has no members).",
		})
	default:
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("Update plan accepted; created %d device plan(s): %s", len(planIDs), strings.Join(planIDs, ", ")),
		})
	}
}

// buildGroupPlanRequest converts the action config into the SDK POST payload. Optional
// fields are sent only when configured (nil pointers are omitted on the wire).
func buildGroupPlanRequest(data PlanActionModel) *pro.ManagedSoftwareUpdatePlanGroupPost {
	cfg := pro.PlanConfigurationPost{
		UpdateAction: data.UpdateAction.ValueString(),
		VersionType:  data.VersionType.ValueString(),
	}
	if v := data.SpecificVersion.ValueStringPointer(); v != nil {
		cfg.SpecificVersion = v
	}
	if v := data.BuildVersion.ValueStringPointer(); v != nil {
		cfg.BuildVersion = v
	}
	if v := data.ForceInstallLocalDateTime.ValueStringPointer(); v != nil {
		cfg.ForceInstallLocalDateTime = v
	}
	if !data.MaxDeferrals.IsNull() && !data.MaxDeferrals.IsUnknown() {
		v := int(data.MaxDeferrals.ValueInt64())
		cfg.MaxDeferrals = &v
	}

	return &pro.ManagedSoftwareUpdatePlanGroupPost{
		Config: cfg,
		Group: pro.PlanGroupPost{
			GroupID:    data.GroupID.ValueString(),
			ObjectType: data.ObjectType.ValueString(),
		},
	}
}
