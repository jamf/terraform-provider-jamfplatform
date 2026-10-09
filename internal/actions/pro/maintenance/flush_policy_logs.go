// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.GetPolicyByID                  (GET    //policies/id/{id} — preflight existence check)
//   proclassic.DeleteLogFlushByLogIDInterval  (DELETE //logflush/policy/id/{id}/interval/{interval}, 201)
//
// Status: current. Last reviewed 2026-08-03.
//
// Flushes a policy's logs older than a chosen age. The age is a two-part
// interval: a quantity drawn from an odd, non-contiguous set (Zero, One, Two,
// Three, Six — there is no Four or Five) and a period (Days, Weeks, Months,
// Years). Wire-probed 2026-08-03 against a live tenant; the endpoint behaves as
// follows, which is why the schema is shaped the way it is:
//
//   - The interval path segment is a single token joining the two parts with
//     "+" — a legacy URL encoding of a space. "Six%20Years" is accepted
//     identically, and the response echoes <interval>6 YEAR</interval>. Users
//     supply the two parts separately and this file joins them, so the encoding
//     never reaches the Terraform configuration surface.
//   - An out-of-set quantity is an unhandled server error, not a validation
//     failure: "Four+Years" returns 500 (a token with no separator at all,
//     e.g. "Bananas", returns 400). Plan-time OneOf validation is therefore the
//     only thing standing between a typo and a 500.
//   - Matching is case-insensitive and the period may be singular or plural
//     ("six+year" is accepted). The canonical title-case plural forms are the
//     only ones offered, because a fixed vocabulary is what makes the docs and
//     the validator agree.
//   - A quantity of Zero means "older than zero <period>", i.e. every log. The
//     schema calls this out because it reads like a no-op and is not.
//   - The {log} segment is NOT validated server-side: both "policy" and
//     "policies" return 201 and are echoed back verbatim. The spec enum says
//     "policy", so that is what is sent.
//   - A nonexistent policy id returns 201 with no error, so the endpoint cannot
//     distinguish "flushed nothing" from "wrong id". Invoke preflights the
//     policy with a read so a typo fails loudly instead of reporting success.

package maintenanceactions

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*FlushPolicyLogsAction)(nil)
var _ action.ActionWithConfigure = (*FlushPolicyLogsAction)(nil)

// logFlushLog is the {log} path segment. Only policy logs are supported.
const logFlushLog = "policy"

// logFlushQuantities is the accepted quantity vocabulary. Deliberately
// non-contiguous — Jamf Pro offers no Four or Five, and anything outside this
// set is a server 500 rather than a validation error.
var logFlushQuantities = []string{"Zero", "One", "Two", "Three", "Six"}

// logFlushPeriods is the accepted period vocabulary.
var logFlushPeriods = []string{"Days", "Weeks", "Months", "Years"}

// markdownValueList renders a slice of enum values as a backticked,
// comma-separated list for MarkdownDescription strings. Deriving the documented
// values from the same slice the OneOf validator uses keeps the docs and the
// validator from drifting apart (tfplugindocs does not render validators, so a
// value list that only lives in a validator is invisible to users).
func markdownValueList(vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = "`" + v + "`"
	}
	return strings.Join(quoted, ", ")
}

// logFlushInterval joins the two user-facing parts into the single interval
// token the endpoint expects. The "+" is the endpoint's own encoding of the
// space in e.g. "Six Months".
func logFlushInterval(quantity, period string) string {
	return quantity + "+" + period
}

// FlushPolicyLogsAction flushes the logs for a policy older than the given interval.
type FlushPolicyLogsAction struct {
	maintenanceAction
}

type FlushPolicyLogsActionModel struct {
	PolicyID types.String         `tfsdk:"policy_id"`
	Quantity types.String         `tfsdk:"quantity"`
	Period   types.String         `tfsdk:"period"`
	Timeouts actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewFlushPolicyLogsAction() action.Action {
	return &FlushPolicyLogsAction{}
}

func (a *FlushPolicyLogsAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_flush_policy_logs"
}

func (a *FlushPolicyLogsAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Flushes a policy's logs that are older than the age given by `quantity` + `period` " +
			"(**Settings → Jamf Pro information → Log flushing** in the Jamf Pro admin UI). " +
			"For example `quantity = \"Six\"` with `period = \"Months\"` flushes logs older than six months. " +
			"Flushing is immediate and cannot be undone. Takes no state." +
			flushPolicyLogsPrivileges,
		Attributes: map[string]actionschema.Attribute{
			"policy_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Jamf Pro policy ID whose logs are flushed. The policy is checked before flushing, because Jamf Pro reports success even for an ID that does not exist.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"quantity": actionschema.StringAttribute{
				Required: true,
				MarkdownDescription: "How many `period` units old a log must be to be flushed. One of " +
					markdownValueList(logFlushQuantities) +
					". Jamf Pro accepts no other quantity, and there is deliberately no `Four` or `Five`. " +
					"**`Zero` flushes every log for the policy** regardless of `period`, because no log is younger than zero days.",
				Validators: []validator.String{
					stringvalidator.OneOf(logFlushQuantities...),
				},
			},
			"period": actionschema.StringAttribute{
				Required: true,
				MarkdownDescription: "The unit `quantity` counts. One of " +
					markdownValueList(logFlushPeriods) + ".",
				Validators: []validator.String{
					stringvalidator.OneOf(logFlushPeriods...),
				},
			},
		},
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *FlushPolicyLogsAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *FlushPolicyLogsAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClassicClient(resp) {
		return
	}

	var data FlushPolicyLogsActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	policyID := data.PolicyID.ValueString()
	interval := logFlushInterval(data.Quantity.ValueString(), data.Period.ValueString())
	age := data.Quantity.ValueString() + " " + data.Period.ValueString()

	// Preflight: the flush endpoint returns success for a policy that does not
	// exist, so without this a typo'd policy_id reports a successful flush.
	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Checking policy %s", policyID)})
	if _, err := a.classic.GetPolicyByID(ctx, policyID); err != nil {
		resp.Diagnostics.AddError(
			"Policy Not Found",
			fmt.Sprintf("Unable to read policy %s, so its logs were not flushed: %s", policyID, helpers.APIErrorDetail(err)),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Flushing logs older than %s for policy %s", age, policyID)})

	if err := a.classic.DeleteLogFlushByLogIDInterval(ctx, logFlushLog, policyID, interval); err != nil {
		resp.Diagnostics.AddError(
			"Flush Policy Logs Failed",
			fmt.Sprintf("Unable to flush logs for policy %s: %s", policyID, helpers.APIErrorDetail(err)),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Flushed logs older than %s for policy %s", age, policyID)})
}
