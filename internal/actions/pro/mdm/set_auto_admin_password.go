// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework-validators/actionvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"golang.org/x/sync/errgroup"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*SetAutoAdminPasswordAction)(nil)
var _ action.ActionWithConfigure = (*SetAutoAdminPasswordAction)(nil)
var _ action.ActionWithConfigValidators = (*SetAutoAdminPasswordAction)(nil)

// SetAutoAdminPasswordAction sets the automatic administrator password on a Mac.
//
// The guid names an account on one computer, and commandData is shared across a
// batch, so one command goes to each computer. A username is resolved to its
// guid on each, through the computer's LAPS accounts (laps_account.go), and the
// targets are checked to be computers first (computers.go). Jamf Pro refuses the command without
// both fields and names what is missing (wire-probed 2026-10-22).
type SetAutoAdminPasswordAction struct {
	mdmAction
}

type SetAutoAdminPasswordActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Username      types.String         `tfsdk:"username"`
	Guid          types.String         `tfsdk:"guid"`
	Password      types.String         `tfsdk:"password"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewSetAutoAdminPasswordAction() action.Action {
	return &SetAutoAdminPasswordAction{}
}

func (a *SetAutoAdminPasswordAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_set_auto_admin_password"
}

func (a *SetAutoAdminPasswordAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := selectorAttributes("computer", "", "")
	attrs["serial_numbers"] = actionschema.ListAttribute{
		Optional:            true,
		ElementType:         types.StringType,
		MarkdownDescription: "Serial numbers of the computers to target (case-sensitive). The action looks up each Management ID first, so `management_ids` saves that step. Set this, `management_ids`, or both.",
		Validators: []validator.List{
			listvalidator.SizeAtLeast(1),
			listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
		},
	}
	attrs["management_ids"] = actionschema.ListAttribute{
		Optional:            true,
		ElementType:         types.StringType,
		MarkdownDescription: "Jamf Pro Management IDs of the computers to target. Use the `id` from the `jamfplatform_devices` or `jamfplatform_device` data sources, or the `members` of a `jamfplatform_device_group` of computers. Every ID must belong to a computer. The action refuses a mobile device before it sends anything. Set this, `serial_numbers`, or both.",
		Validators: []validator.List{
			listvalidator.SizeAtLeast(1),
			listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
		},
	}
	attrs["username"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Short name of the local administrator account to set the password on. The action looks up the account's GUID on each computer, because the same account has a different GUID on every Mac. Only accounts that Jamf Pro manages through LAPS resolve. For any other account, set `guid`. Set exactly one of `username` or `guid`.",
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
	attrs["guid"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "GUID of the local administrator account to set the password on. A GUID identifies one account on one computer, so this works with a single target only. Jamf Pro does not check it and queues the command for any value, so a wrong GUID fails silently on the Mac. Prefer `username`. To find a GUID, run `dscl . -read /Users/<name> GeneratedUID` on the Mac. Set exactly one of `username` or `guid`.",
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
	attrs["password"] = actionschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The new password. Every targeted computer gets this value. Jamf Pro never returns it." + secretAttrNote,
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Sets the automatic administrator password on one or more Macs. The action sends one command per computer. If a computer already has this command waiting, Jamf Pro keeps the waiting command and queues no other, so a password you change in the meantime never reaches that computer.\n\n" +
			"Every computer you target gets the **same** password. Use that for a one-off baseline or a recovery. Anyone who learns the password has admin on every Mac in the list, so leave routine rotation to Jamf Pro's LAPS, which gives each computer its own." + setAutoAdminPasswordPrivileges,
		Attributes: attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *SetAutoAdminPasswordAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return append(deviceTargetListConfigValidators(),
		actionvalidator.ExactlyOneOf(path.MatchRoot("username"), path.MatchRoot("guid")),
	)
}

func (a *SetAutoAdminPasswordAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

// lapsLookupConcurrency caps the accounts lookups in flight, per the repository's
// load budget for one Jamf Pro tenant.
const lapsLookupConcurrency = 5

func (a *SetAutoAdminPasswordAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data SetAutoAdminPasswordActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ids, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}
	ids = uniqueInOrder(ids)

	if helpers.IsConfiguredValue(data.Guid) && len(ids) != 1 {
		resp.Diagnostics.AddError(
			"GUID Needs A Single Computer",
			fmt.Sprintf("guid names one account on one computer, but %d computers are targeted. Set username instead, which is resolved on each computer.", len(ids)),
		)
		return
	}

	if !a.requireComputers(ctx, resp, ids) {
		return
	}

	guids := make(map[string]string, len(ids))
	if helpers.IsConfiguredValue(data.Guid) {
		guids[ids[0]] = data.Guid.ValueString()
	} else if !a.resolveLAPSAccountGUIDs(ctx, resp, ids, data.Username.ValueString(), guids) {
		return
	}

	queued := 0
	for _, id := range ids {
		data.Guid = types.StringValue(guids[id])
		if a.sendCommand(ctx, resp, id, setAutoAdminPasswordCommand(data), "Set Auto Admin Password") {
			queued++
		}
	}
	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Set Auto Admin Password queued for %d of %d computer(s)", queued, len(ids))})
}

// resolveLAPSAccountGUIDs fills guids for every computer, or reports every one
// that failed in a single error naming each, and returns false. Nothing is sent
// when any lookup fails: a group is meant to be commanded as a whole, and the
// computer that lacks the account is the one the operator needs to see.
func (a *SetAutoAdminPasswordAction) resolveLAPSAccountGUIDs(ctx context.Context, resp *action.InvokeResponse, ids []string, username string, guids map[string]string) bool {
	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Resolving account %s on %d computer(s)", username, len(ids))})

	var (
		mu       sync.Mutex
		problems []string
	)
	group, gctx := errgroup.WithContext(ctx)
	group.SetLimit(lapsLookupConcurrency)
	for _, id := range ids {
		group.Go(func() error {
			guid, problem := a.lookupLAPSAccountGUID(gctx, id, username)
			mu.Lock()
			defer mu.Unlock()
			if problem != "" {
				problems = append(problems, id+": "+problem)
				return nil
			}
			guids[id] = guid
			return nil
		})
	}
	_ = group.Wait()

	if len(problems) > 0 {
		sort.Strings(problems)
		resp.Diagnostics.AddError(
			"Account Not Resolved",
			fmt.Sprintf("Could not resolve account %q on %d of %d computer(s), so nothing was sent. Only accounts that Jamf Pro manages through LAPS resolve. For any other account, set guid and target a single computer.\n\n%s",
				username, len(problems), len(ids), strings.Join(problems, "\n")),
		)
		return false
	}
	return true
}

// uniqueInOrder drops repeated ids, keeping first occurrence order. A repeat
// would queue the same command twice for one computer, and Jamf Pro keeps only
// the first while it waits.
func uniqueInOrder(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func setAutoAdminPasswordCommand(data SetAutoAdminPasswordActionModel) *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType: string(pro.MDMCommandTypeSetAutoAdminPassword),
		SETAUTOADMINPASSWORD: &pro.SetAutoAdminPasswordCommand{
			CommandType: pro.MDMCommandTypeSetAutoAdminPassword,
			Guid:        data.Guid.ValueStringPointer(),
			Password:    data.Password.ValueStringPointer(),
		},
	}
}
