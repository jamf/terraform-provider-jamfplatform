// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//
//	securitycloud.DeployActivationProfileToUemV1
//
// Status: current. Last reviewed 2026-08-29.
package uemconnectactions

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var (
	_ action.Action              = (*DeployActivationProfileAction)(nil)
	_ action.ActionWithConfigure = (*DeployActivationProfileAction)(nil)
)

// DeployActivationProfileAction deploys an activation profile's configuration
// profiles to Jamf Pro.
type DeployActivationProfileAction struct {
	uemConnectAction
}

// DeployActivationProfileActionModel is the action's configuration.
type DeployActivationProfileActionModel struct {
	ActivationProfileCode types.String `tfsdk:"activation_profile_code"`
	OS                    types.String `tfsdk:"os"`
	JamfProGroupIDs       types.Set    `tfsdk:"jamf_pro_group_ids"`
}

// NewDeployActivationProfileAction returns a new instance of
// DeployActivationProfileAction.
func NewDeployActivationProfileAction() action.Action {
	return &DeployActivationProfileAction{}
}

// Metadata sets the action type name for the Terraform provider.
func (a *DeployActivationProfileAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_activation_profile_deploy"
}

// Schema returns the action schema.
func (a *DeployActivationProfileAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**\"Deploy to Jamf Pro\"** under **UEM actions** on a Jamf Security Cloud " +
			"activation profile. Creates the activation profile's configuration profile in Jamf Pro for one " +
			"operating system, and scopes it to the Jamf Pro groups you name.\n\n" +
			"Re-running it is safe: deploying the same activation profile and operating system again updates " +
			"the configuration profile Jamf Security Cloud already created rather than adding a second one, " +
			"and it recreates the configuration profile if it has since been deleted in Jamf Pro.\n\n" +
			"~> **Scope only ever accumulates.** `jamf_pro_group_ids` adds groups to the configuration " +
			"profile's scope; it never removes one. Deploying group `3` to a profile already scoped to groups " +
			"`1` and `2` leaves it scoped to all three, and deploying with the argument omitted leaves the " +
			"existing scope untouched. To narrow or clear the scope, edit the configuration profile in Jamf " +
			"Pro.\n\n" +
			"~> **Omitting `jamf_pro_group_ids` on a first deployment scopes the configuration profile to " +
			"nothing**, so it reaches no devices. Jamf Security Cloud reports success either way, so name at " +
			"least one group unless you intend to scope the profile in Jamf Pro yourself." +
			deployActivationProfilePrivileges,
		Attributes: map[string]actionschema.Attribute{
			"activation_profile_code": actionschema.StringAttribute{
				MarkdownDescription: "The code of the activation profile to deploy. Jamf Security Cloud " +
					"issues it during activation profile setup; it is the last path segment when you open the " +
					"activation profile's deployment page in the Jamf Security Cloud console. For a profile " +
					"Terraform manages, use `jamfplatform_security_cloud_activation_profile.<name>.id` instead. " +
					"There is no way to look a code up by profile name.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"os": actionschema.StringAttribute{
				MarkdownDescription: "**\"Select your OS\"**: which of the activation profile's " +
					"configuration profiles to deploy. One deployment per operating system: to cover more than " +
					"one, invoke this action once for each.\n\n" +
					"Valid values: " + markdownValueList(osValues()) + ".",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(osValues()...),
				},
			},
			"jamf_pro_group_ids": actionschema.SetAttribute{
				MarkdownDescription: "**\"Optionally select UEM group\"**: the Jamf Pro groups to add to " +
					"the deployed configuration profile's scope, as group IDs.\n\n" +
					"Computer groups when `os` is `" + macOSValue + "`, mobile device groups otherwise. A " +
					"group of the wrong kind for the chosen `os`, or one that does not exist, is refused.\n\n" +
					"Read the warnings above before relying on this: scope accumulates and is never cleared " +
					"here, and omitting the argument on a first deployment leaves the configuration profile " +
					"scoped to nothing.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(
						stringvalidator.RegexMatches(
							jamfProGroupIDPattern,
							"must be a Jamf Pro group ID — digits only, with no `computer_` or `mobile_` prefix",
						),
					),
				},
			},
		},
	}
}

// Configure wires the Jamf Security Cloud client into the action.
func (a *DeployActivationProfileAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

// Invoke deploys the activation profile's configuration profile to Jamf Pro.
//
// Fire-once with nothing to poll: the deploy is synchronous and reports only
// success, so there is no run to follow and no identifier returned. What it
// produced is readable from the integration afterwards, but only from a field the
// SDK does not yet model, so this action reports the deployment in its progress
// messages instead.
//
// Idempotent in the useful sense, both wire-verified 2026-08-29: a repeat of the
// same activation profile and operating system updates the existing configuration
// profile rather than creating a second, and a repeat after the configuration
// profile was deleted in Jamf Pro recreates it.
func (a *DeployActivationProfileAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClient(resp) {
		return
	}

	var data DeployActivationProfileActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groups := groupIDsFromSet(ctx, data.JamfProGroupIDs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	code := data.ActivationProfileCode.ValueString()
	osValue := data.OS.ValueString()

	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Deploying activation profile %s (%s) to Jamf Pro", code, osValue),
	})

	request := &securitycloud.ActivationProfileDeployRequest{
		Platform: osToWire[osValue],
		Uem:      uemJamfPro,
	}
	if len(groups) > 0 {
		request.UemGroups = &groups
	}

	if err := a.client.DeployActivationProfileToUemV1(ctx, code, request); err != nil {
		if !appendDeployDiagnostics(&resp.Diagnostics, err, code, osValue, groups) {
			resp.Diagnostics.AddError(
				"Activation Profile Deploy Failed",
				fmt.Sprintf("Unable to deploy activation profile %s (%s) to Jamf Pro: %s", code, osValue, helpers.APIErrorDetail(err)),
			)
		}
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{
		Message: deployedMessage(code, osValue, groups),
	})
}

// groupIDsFromSet converts the configured group IDs to the order-independent slice
// the request carries.
//
// Sorted so the request body is stable across runs for the same configuration: the
// server ignores order, but an unstable body makes a captured request harder to
// compare against the next one. A null or unknown set yields nothing, which the
// caller turns into an omitted field.
func groupIDsFromSet(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	var groups []string
	diags.Append(set.ElementsAs(ctx, &groups, false)...)
	if diags.HasError() {
		return nil
	}
	sort.Strings(groups)
	return groups
}

// deployedMessage describes what the deploy did, naming the groups so the progress
// output records the scope that was added rather than only that something was.
//
// The no-groups line names both outcomes because nothing here can tell them apart:
// Invoke reads no server state, so it does not know whether a configuration profile
// already existed. "Unchanged" alone would be true of a repeat and false of a first
// deployment, which creates a configuration profile scoped to nothing — the case the
// schema's second warning exists for, and the one an operator scanning apply output
// would otherwise read as nothing to act on.
func deployedMessage(code, osValue string, groups []string) string {
	if len(groups) == 0 {
		return fmt.Sprintf(
			"Deployed activation profile %s (%s) to Jamf Pro; no groups were named, so an existing "+
				"configuration profile keeps the scope it had, and a newly created one is scoped to "+
				"nothing and reaches no devices", code, osValue)
	}
	return fmt.Sprintf(
		"Deployed activation profile %s (%s) to Jamf Pro and added %s to the configuration profile's scope",
		code, osValue, strings.Join(groups, ", "))
}

// appendDeployDiagnostics turns a deploy failure into the most specific diagnostic
// the error body supports, and reports whether it recognised one.
//
// CONNECTOR_MISCONFIGURED is the one that matters. Jamf Security Cloud sends it,
// with the description "UEM is misconfigured", for a group ID that does not exist
// and for one belonging to the wrong kind of group for the chosen operating system
// — wire-verified 2026-08-29, where computer group 29 was refused under an iOS
// value and accepted under macos. Nothing is misconfigured in either case, and the
// message names neither the field nor the group, so it is worth every word of the
// replacement.
func appendDeployDiagnostics(diags *diag.Diagnostics, err error, code, osValue string, groups []string) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}

	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeActivationProfileNotFound:
			diags.AddError(
				"Activation profile not found",
				fmt.Sprintf("Jamf Security Cloud has no activation profile with code %q that can be deployed "+
					"for `os = %q`. Either the code is wrong, or that activation profile has no configuration "+
					"profile for this operating system — Jamf Security Cloud does not distinguish the two. "+
					"Check the code against the activation profile's deployment page in the Jamf Security "+
					"Cloud console, and that the operating system is one that activation profile covers. "+
					"Reported by Jamf Security Cloud: %s", code, osValue, detail.Description),
			)
		case codeConnectorMisconfigured:
			diags.AddError(
				"A named Jamf Pro group cannot be used for this deployment",
				groupProblemDetail(osValue, groups, detail.Description),
			)
		case codeConnectorDisabled:
			diags.AddError(
				"UEM Connect is disabled, so it cannot deploy",
				"Jamf Security Cloud refuses to deploy through a disabled integration. Enable it first — set "+
					"`enabled = true` on the jamfplatform_security_cloud_uem_connect resource, or enable it in "+
					"the Jamf Security Cloud admin UI. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeConnectorNotConnected:
			diags.AddError(
				"UEM Connect is not connected to Jamf Pro",
				"Jamf Security Cloud cannot deploy configuration profiles until its integration has connected "+
					"to Jamf Pro. Check the integration's Jamf Pro credentials, and that its first device sync "+
					"has completed. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeMultipleActivationProfiles:
			diags.AddError(
				"More than one activation profile is active, so the deployment target is ambiguous",
				"This tenant has several active activation profiles and Jamf Security Cloud cannot tell which "+
					"one to deploy. Nothing in this provider can list them; resolve it in the Jamf Security "+
					"Cloud console by deactivating the activation profiles you are not deploying. Reported by "+
					"Jamf Security Cloud: "+detail.Description,
			)
		case codeValidationFailed:
			if !strings.Contains(detail.Description, invalidGroupIDMarker) {
				continue
			}
			diags.AddError(
				"A named Jamf Pro group ID is not a number",
				"`jamf_pro_group_ids` takes Jamf Pro group IDs as plain digits. The `computer_` and `mobile_` "+
					"prefixes belong to the group mapping on the jamfplatform_security_cloud_uem_connect "+
					"resource, not here — the kind of group is decided by `os`. Reported by Jamf Security "+
					"Cloud: "+detail.Description,
			)
		case codeNotEntitled:
			addNotEntitled(diags, detail.Description)
		default:
			continue
		}
		matched = true
	}
	return matched
}

// groupProblemDetail explains a rejected group ID, naming the kind of group the
// chosen operating system takes.
//
// The server cannot say which group it objected to, so this names all of them and
// the two things wrong with a group ID that produce this refusal.
func groupProblemDetail(osValue string, groups []string, description string) string {
	kind := "mobile device groups"
	other := "computer group"
	if osValue == macOSValue {
		kind = "computer groups"
		other = "mobile device group"
	}

	named := "no groups were named"
	if len(groups) > 0 {
		named = "named: " + strings.Join(groups, ", ")
	}

	return fmt.Sprintf(
		"Jamf Security Cloud refused the deployment because one of the Jamf Pro groups cannot be used, but "+
			"does not say which. With `os = %q` the deployment scopes to %s, so a %s here is refused, as is a "+
			"group ID that does not exist. Groups %s. Reported by Jamf Security Cloud: %s",
		osValue, kind, other, named, description)
}

// markdownValueList renders accepted values as a backticked, comma-separated list
// for a MarkdownDescription.
func markdownValueList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "`"+value+"`")
	}
	return strings.Join(quoted, ", ")
}
