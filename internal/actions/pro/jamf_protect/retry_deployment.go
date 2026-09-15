// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListJamfProtectDeploymentTasksV1    (GET  /v1/jamf-protect/deployments/{id}/tasks — paginated task search)
//   pro.RetryJamfProtectDeploymentTasksV1   (POST /v1/jamf-protect/deployments/{id}/tasks/retry — task IDs body, 204)
//
// Status: current. Last reviewed 2026-07-03.
//
// Retries failed Jamf Protect install tasks for a deployment. The retry API
// takes deployment *task* IDs (not computer IDs), so every mode resolves to a
// set of task IDs and POSTs them. Three mutually-exclusive target modes
// (enforced by exactlyOneTargetValidator):
//
//	computer  — serial_number / management_id / udid → resolve to a Jamf Pro computer
//	            id → retry that computer's task(s) (failed only unless only_failed=false).
//	task_ids  — retry an explicit set of deployment task IDs (raw passthrough, no GET).
//	all_failed — retry every failed task in the deployment (mirrors the UI "Retry Failed").
//
// The deployment id is the deployment UUID surfaced by the
// jamfplatform_pro_jamf_protect_plans data source (plans[*].uuid), not the
// integer configuration-profile id.
//
// Live-confirmed API behavior that the developer docs get wrong (do not "fix"
// these back):
//   - The status RSQL filter is server-broken on this endpoint: it validates
//     against an unrelated enum [Installed, Pending, Failed, Acknowledged]
//     (status==GAVE_UP → 400), and even a validation-passing value 500s when
//     applied. So we NEVER send filter=status==...; we fetch tasks unfiltered
//     and match client-side. (Filtering other fields works, so it is specific
//     to status.)
//   - A failed task's response status IS "GAVE_UP" (the response enum the docs
//     list is correct — only the filter enum diverges). Match with EqualFold.
//   - There is no server-side computerId filter, so computer matching is
//     client-side too.
//   - id/computerId come back as JSON numbers; the SDK decodes them as
//     *json.Number (deployment_task.id/computerId override), coerced to string
//     via numStr for the retry POST body and computer matching.

package jamfprotectactions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/computertarget"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// statusFailed is the DeploymentTask.status response value for a failed install
// task, matched client-side with EqualFold. Live-confirmed; the response enum
// (unlike the broken status filter) uses this value. Other observed values:
// VERIFIED_INSTALL / COMPLETE (success), INSTALL_IN_PROGRESS (pending).
const statusFailed = pro.DeploymentTaskStatusGaveUp

var _ action.Action = (*RetryDeploymentAction)(nil)
var _ action.ActionWithConfigure = (*RetryDeploymentAction)(nil)
var _ action.ActionWithConfigValidators = (*RetryDeploymentAction)(nil)

// RetryDeploymentAction retries failed Jamf Protect deployment install tasks.
type RetryDeploymentAction struct {
	jamfProtectAction
}

// RetryDeploymentActionModel is the action configuration.
type RetryDeploymentActionModel struct {
	DeploymentID types.String `tfsdk:"deployment_id"`
	SerialNumber types.String `tfsdk:"serial_number"`
	ManagementID types.String `tfsdk:"management_id"`
	UDID         types.String `tfsdk:"udid"`
	TaskIDs      types.List   `tfsdk:"task_ids"`
	AllFailed    types.Bool   `tfsdk:"all_failed"`
	OnlyFailed   types.Bool   `tfsdk:"only_failed"`
}

// NewRetryDeploymentAction constructs the action.
func NewRetryDeploymentAction() action.Action {
	return &RetryDeploymentAction{}
}

func (a *RetryDeploymentAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_jamf_protect_deployment_retry"
}

func (a *RetryDeploymentAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Retries failed Jamf Protect install tasks for a deployment (Settings → Jamf apps → Jamf Protect → deployment → Retry). " +
			"Specify exactly one target: a computer (`serial_number`, `management_id`, or `udid`), an explicit `task_ids` list, or `all_failed = true`. " +
			"Resolving a computer identifier also requires the **Inventory → Devices → Read** permission in Jamf Account (API capability `devices:read`). Takes no state." +
			retryDeploymentPrivileges,
		Attributes: map[string]actionschema.Attribute{
			"deployment_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The Jamf Protect deployment UUID. This is the `uuid` of a plan reported by the `jamfplatform_pro_jamf_protect_plans` data source (not the integer configuration-profile id).",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"serial_number": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Serial number of the computer to retry (case-sensitive). Retries that computer's deployment task(s). Provide this, `management_id`, or `udid`.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.MatchRoot("management_id"), path.MatchRoot("udid")),
				},
			},
			"management_id": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Jamf Pro Management ID (UUID) of the computer to retry, as reported by the `jamfplatform_devices`/`jamfplatform_device` data sources. Provide this, `serial_number`, or `udid`.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.MatchRoot("serial_number"), path.MatchRoot("udid")),
				},
			},
			"udid": actionschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Hardware UDID of the computer to retry. Retries that computer's deployment task(s). Provide this, `serial_number`, or `management_id`.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.MatchRoot("serial_number"), path.MatchRoot("management_id")),
				},
			},
			"task_ids": actionschema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Explicit deployment task IDs to retry (the `id` values returned by the deployment's task search). Advanced escape hatch. Mutually exclusive with the computer selector and `all_failed`.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"all_failed": actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Retry every failed task in the deployment. Mirrors the admin UI \"Retry Failed\" button. It can re-queue installs across many computers. Mutually exclusive with the computer selector and `task_ids`.",
			},
			"only_failed": actionschema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Only meaningful with the computer selector. When `true` (the default), retries only the computer's failed task(s); when `false`, retries all of that computer's tasks regardless of status. Ignored for `task_ids` and `all_failed`.",
			},
		},
	}
}

func (a *RetryDeploymentAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

// ConfigValidators wires the exactly-one-target contract.
func (a *RetryDeploymentAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return []action.ConfigValidator{exactlyOneTargetValidator{}}
}

func (a *RetryDeploymentAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClient(resp) {
		return
	}

	var data RetryDeploymentActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deploymentID := data.DeploymentID.ValueString()

	var taskIDs []string
	switch {
	case helpers.IsConfiguredValue(data.SerialNumber) || helpers.IsConfiguredValue(data.ManagementID) || helpers.IsConfiguredValue(data.UDID):
		computerID, ok := computertarget.ResolveComputerID(ctx, a.client, resp, data.ManagementID, data.SerialNumber, data.UDID)
		if !ok {
			return
		}

		onlyFailed := data.OnlyFailed.IsNull() || data.OnlyFailed.ValueBool()

		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Listing deployment %s tasks for computer %s", deploymentID, computerID)})
		tasks, err := a.listAllTasks(ctx, deploymentID)
		if err != nil {
			resp.Diagnostics.AddError("Jamf Protect Deployment Retry Failed", listTasksErr(deploymentID, err))
			return
		}
		for _, tk := range tasks {
			if numStr(tk.ComputerID) != computerID {
				continue
			}
			if onlyFailed && !strings.EqualFold(tk.Status, statusFailed) {
				continue
			}
			taskIDs = append(taskIDs, numStr(tk.ID))
		}
		if len(taskIDs) == 0 {
			scope := "failed"
			if !onlyFailed {
				scope = "matching"
			}
			resp.Diagnostics.AddError(
				"No Retryable Task Found",
				fmt.Sprintf("No %s deployment task found for computer %s in deployment %s. Check that deployment_id is the plan UUID and that the computer is scoped to the plan.", scope, computerID, deploymentID),
			)
			return
		}

	case !data.TaskIDs.IsNull():
		resp.Diagnostics.Append(data.TaskIDs.ElementsAs(ctx, &taskIDs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

	case data.AllFailed.ValueBool():
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Listing failed tasks in deployment %s", deploymentID)})
		tasks, err := a.listAllTasks(ctx, deploymentID)
		if err != nil {
			resp.Diagnostics.AddError("Jamf Protect Deployment Retry Failed", listTasksErr(deploymentID, err))
			return
		}
		for _, tk := range tasks {
			if strings.EqualFold(tk.Status, statusFailed) {
				taskIDs = append(taskIDs, numStr(tk.ID))
			}
		}
		if len(taskIDs) == 0 {
			resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("No failed tasks to retry in deployment %s", deploymentID)})
			return
		}

	default:
		// exactlyOneTargetValidator guarantees a mode is selected; this is a
		// defensive guard only.
		resp.Diagnostics.AddError(
			"Missing Retry Target",
			"Specify exactly one of: serial_number/management_id/udid, task_ids, or all_failed = true.",
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Requesting retry of %d task(s) in deployment %s", len(taskIDs), deploymentID)})
	if err := a.client.RetryJamfProtectDeploymentTasksV1(ctx, deploymentID, &pro.Ids{IDs: &taskIDs}); err != nil {
		resp.Diagnostics.AddError("Jamf Protect Deployment Retry Failed", retryErr(deploymentID, err))
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Retry requested for %d task(s) in deployment %s", len(taskIDs), deploymentID)})
}

// listAllTasks fetches every task for the deployment UNFILTERED. The status RSQL
// filter is server-broken on this endpoint (see the package doc), so all status
// matching happens client-side; the SDK paginates internally.
func (a *RetryDeploymentAction) listAllTasks(ctx context.Context, deploymentID string) ([]pro.DeploymentTask, error) {
	return a.client.ListJamfProtectDeploymentTasksV1(ctx, deploymentID, nil, "")
}

// numStr renders a *json.Number id/computerId (the wire returns JSON numbers) as
// its string form; nil becomes "".
func numStr(n *json.Number) string {
	if n == nil {
		return ""
	}
	return n.String()
}

// listTasksErr renders the shared deployment-task-listing error message.
func listTasksErr(deploymentID string, err error) string {
	return fmt.Sprintf("Unable to list tasks for deployment %s: %s. Check that deployment_id is a valid plan UUID (see the jamfplatform_pro_jamf_protect_plans data source).", deploymentID, err)
}

// retryErr renders the retry-POST error, calling out the specific 400 meaning
// (the tenant is not registered with Jamf Protect) rather than a generic status.
func retryErr(deploymentID string, err error) string {
	var apiErr *jamfplatform.APIResponseError
	if errors.As(err, &apiErr) && apiErr.HasStatus(400) {
		return fmt.Sprintf(
			"Unable to retry deployment %s: the Cloud Services Connection has not been established — this tenant is not registered with Jamf Protect (see jamfplatform_pro_jamf_protect). Original error: %s",
			deploymentID, err,
		)
	}
	return fmt.Sprintf("Unable to retry deployment %s: %s", deploymentID, err)
}

// exactlyOneTargetValidator enforces that exactly one retry target mode is
// configured: a computer selector (serial_number/management_id), an explicit
// task_ids list, or all_failed=true. serial_number vs management_id exclusivity
// is handled by ConflictsWith on those attributes.
type exactlyOneTargetValidator struct{}

func (v exactlyOneTargetValidator) Description(context.Context) string {
	return "exactly one retry target must be specified: a computer (serial_number/management_id), task_ids, or all_failed = true"
}

func (v exactlyOneTargetValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v exactlyOneTargetValidator) ValidateAction(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var data RetryDeploymentActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Defer while any target-selecting value is unknown (resolved at apply).
	if data.SerialNumber.IsUnknown() || data.ManagementID.IsUnknown() || data.UDID.IsUnknown() || data.TaskIDs.IsUnknown() || data.AllFailed.IsUnknown() {
		return
	}

	hasComputer := helpers.IsConfiguredValue(data.SerialNumber) || helpers.IsConfiguredValue(data.ManagementID) || helpers.IsConfiguredValue(data.UDID)
	hasTaskIDs := !data.TaskIDs.IsNull() && len(data.TaskIDs.Elements()) > 0
	hasAllFailed := data.AllFailed.ValueBool()

	modes := 0
	for _, on := range []bool{hasComputer, hasTaskIDs, hasAllFailed} {
		if on {
			modes++
		}
	}

	switch {
	case modes == 0:
		resp.Diagnostics.AddError(
			"Missing Retry Target",
			"Specify exactly one of: serial_number/management_id, task_ids, or all_failed = true.",
		)
	case modes > 1:
		resp.Diagnostics.AddError(
			"Conflicting Retry Targets",
			"Specify only one retry target: a computer (serial_number/management_id), task_ids, or all_failed = true.",
		)
	}
}
