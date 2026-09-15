// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package mdmactions holds the Jamf Pro MDM command actions. Three remain:
// send_blank_push, renew_mdm_profile and flush_mdm_commands.
//
// Fourteen others were removed at the Platform API GA — device_lock, the lost
// mode trio, the remote desktop pair, clear_restrictions_password,
// clear_passcode, delete_user, log_out_user, unlock_user_account,
// set_auto_admin_password and the enhanced-log-collection pair — because
// POST /v2/mdm/commands was withdrawn. Only the send verb went: the GET side of
// /v1 and /v2 survives, which is why the three above are unaffected.
//
// Why there is no replacement, recorded here because it is the first question
// anyone will ask. The capability did not go away with the endpoint: the Classic
// API still serves these commands on both /computercommands and
// /mobiledevicecommands, and Jamf Pro v1872 actively *re-privileged* the five
// mobile-device command POSTs (destructive-device-actions:execute and
// device-actions:execute), which is not what a withdrawal looks like. Rebuilding
// on Classic is therefore viable but is a different construct surface, not a
// port: the command name is path-encoded rather than carried in the body, targets
// are id lists rather than a clientData array, and the payload is XML. It wants
// its own change with its own wire probes rather than being smuggled into a
// removal.
package mdmactions

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/actionvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	devSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is empty: these management commands ride continuously
// deployed Jamf Pro endpoints with no meaningful version floor.
const minJamfProVersion = ""

// blankPushBatchNote documents send_blank_push's batch semantics. That endpoint
// reports per-device outcomes: it accepts the request and names the devices that
// failed, rather than failing wholesale.
const blankPushBatchNote = "\n\n" +
	"All targeted devices are pushed in a single request. Devices that do not accept the push " +
	"are reported individually as a warning; the invocation itself still succeeds."

// mdmAction shares Configure logic across the MDM command actions. It holds the
// three client surfaces the package needs: the Jamf Pro client (blank-push,
// renew-profile), the Platform devices client (serial-number resolution, since a
// Platform device id is the Jamf Pro managementId), and the ProClassic client
// (command-queue flush).
type mdmAction struct {
	client  *pro.Client
	classic *proclassic.Client
	devices *devSDK.Client
}

// configure binds the provider-supplied clients to the action. Every remaining
// action in this package shares the empty version floor, so there is no
// per-action override: the two enhanced-log-collection actions that needed one
// (11.30) went with POST /v2/mdm/commands at the Platform API GA.
func (a *mdmAction) configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *providerdata.Data, got %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "mdm")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The classic client is bound unconditionally because mdmAction is shared, but
	// the floor is deliberately NOT applied to it: only flush_mdm_commands uses it,
	// and that command has no version requirement of its own.
	classic, cdiags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "mdm")
	resp.Diagnostics.Append(cdiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	a.client = client
	a.classic = classic
	a.devices = devSDK.New(pd.Client)
}

// ensureClient guarantees the Jamf Pro client was configured before Invoke.
func (a *mdmAction) ensureClient(resp *action.InvokeResponse) bool {
	if a.client != nil {
		return true
	}
	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Platform client was not configured. Re-run terraform init/apply so the provider can configure successfully.",
	)
	return false
}

// ensureClassicClient guarantees the ProClassic client was configured before Invoke.
func (a *mdmAction) ensureClassicClient(resp *action.InvokeResponse) bool {
	if a.classic != nil {
		return true
	}
	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Platform client was not configured. Re-run terraform init/apply so the provider can configure successfully.",
	)
	return false
}

// serialFilterBudgetBytes bounds the URL-encoded length of the RSQL filter
// expression built for one bulk serial-resolution request.
//
// The Platform device inventory rejects any request line longer than 8192 bytes
// ("Line exceeds limit of 8192 bytes"). That ceiling covers the whole request
// line — path, tenant id, page and page-size parameters and the encoded filter
// — so the filter itself gets a deliberately conservative share of it.
//
// The limit applies to the ENCODED form, which is why chunking measures encoded
// bytes: every quote and comma costs three bytes (%22, %2C), so budgeting
// against raw serial length would overshoot by roughly half.
const serialFilterBudgetBytes = 4096

// resolveManagementIDs collects every targeted client management id for
// send_blank_push. Both selectors are additive: management ids are taken
// verbatim and serial numbers are resolved through the Platform devices
// inventory, whose device id IS the Jamf Pro managementId.
//
// Duplicates are preserved rather than collapsed: the config is taken at its
// word.
func (a *mdmAction) resolveManagementIDs(ctx context.Context, resp *action.InvokeResponse, managementIDsAttr, serialNumbersAttr types.List) ([]string, bool) {
	var ids []string

	if !managementIDsAttr.IsNull() && !managementIDsAttr.IsUnknown() {
		resp.Diagnostics.Append(managementIDsAttr.ElementsAs(ctx, &ids, false)...)
		if resp.Diagnostics.HasError() {
			return nil, false
		}
	}

	if !serialNumbersAttr.IsNull() && !serialNumbersAttr.IsUnknown() {
		var serials []string
		resp.Diagnostics.Append(serialNumbersAttr.ElementsAs(ctx, &serials, false)...)
		if resp.Diagnostics.HasError() {
			return nil, false
		}
		resolved, ok := a.resolveSerialNumbers(ctx, resp, serials)
		if !ok {
			return nil, false
		}
		ids = append(ids, resolved...)
	}

	if len(ids) == 0 {
		resp.Diagnostics.AddError(
			"Missing Device Identifier",
			"Specify at least one of management_ids or serial_numbers to select the devices.",
		)
		return nil, false
	}

	return ids, true
}

// resolveSerialNumbers maps serial numbers to client management ids in bulk,
// preserving the caller's ordering.
//
// The Platform device inventory accepts an RSQL `in=` set on serialNumber, so N
// serials resolve in ceil(N/chunk) requests rather than one request per serial
// — which is what the SDK's ResolveDeviceIDBySerialNumber does, issuing a
// single-equality filtered GET each time. Chunk size is driven by
// serialFilterBudgetBytes; the SDK paginates within a chunk.
//
// Matching is re-applied client-side rather than trusted to the server-side
// filter, so a serial only counts when it comes back exactly. Every serial that
// does not resolve is named, which is a strict improvement on the batched
// command POST itself: that fails opaquely without identifying any device.
func (a *mdmAction) resolveSerialNumbers(ctx context.Context, resp *action.InvokeResponse, serials []string) ([]string, bool) {
	chunks := chunkSerialsByEncodedSize(serials, serialFilterBudgetBytes)

	found := make(map[string]string, len(serials))
	ambiguous := make(map[string]bool)

	for i, chunk := range chunks {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf(
			"Resolving %d serial number(s) (request %d of %d)", len(chunk), i+1, len(chunks))})

		quoted := make([]string, 0, len(chunk))
		for _, serial := range chunk {
			quoted = append(quoted, quoteRSQLValue(serial))
		}
		filter := "serialNumber=in=(" + strings.Join(quoted, ",") + ")"

		devices, err := a.devices.ListDevices(ctx, nil, filter)
		if err != nil {
			resp.Diagnostics.AddError(
				"Device Lookup Failed",
				fmt.Sprintf("Unable to resolve %d serial number(s) to management ids: %s", len(chunk), helpers.APIErrorDetail(err)),
			)
			return nil, false
		}

		for _, device := range devices {
			if _, seen := found[device.SerialNumber]; seen {
				ambiguous[device.SerialNumber] = true
				continue
			}
			found[device.SerialNumber] = device.ID
		}
	}

	if len(ambiguous) > 0 {
		resp.Diagnostics.AddError(
			"Ambiguous Serial Number",
			fmt.Sprintf("More than one device reports the serial number(s) %s, so the intended target is unclear. Use management_ids to select the devices explicitly.",
				strings.Join(sortedKeys(ambiguous), ", ")),
		)
		return nil, false
	}

	ids := make([]string, 0, len(serials))
	var missing []string
	for _, serial := range serials {
		id, ok := found[serial]
		if !ok {
			missing = append(missing, serial)
			continue
		}
		ids = append(ids, id)
	}

	if len(missing) > 0 {
		resp.Diagnostics.AddError(
			"Device Not Found",
			fmt.Sprintf("No device matched the serial number(s) %s. Serial numbers are case-sensitive.",
				strings.Join(missing, ", ")),
		)
		return nil, false
	}

	return ids, true
}

// chunkSerialsByEncodedSize splits serials into groups whose encoded RSQL `in=`
// argument stays within budget. A single serial that exceeds the budget on its
// own still forms a chunk — there is no smaller request to make.
func chunkSerialsByEncodedSize(serials []string, budget int) [][]string {
	var (
		chunks  [][]string
		current []string
		size    int
	)
	const separatorCost = len("%2C") // an encoded comma between arguments

	for _, serial := range serials {
		cost := len(url.QueryEscape(quoteRSQLValue(serial))) + separatorCost
		if len(current) > 0 && size+cost > budget {
			chunks = append(chunks, current)
			current, size = nil, 0
		}
		current = append(current, serial)
		size += cost
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// quoteRSQLValue renders a value as a quoted RSQL argument. It mirrors the
// escaping the SDK applies in FormatArgument, which lives in an internal
// package and cannot be imported here.
func quoteRSQLValue(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// sortedKeys returns the map's keys in a stable order so diagnostics do not
// vary between runs.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// targetListAttributes returns the shared management_ids / serial_numbers
// selector used by the one surviving command action that targets devices in bulk.
// deviceNoun tunes the description (e.g. "computer", "mobile device").
//
// These are lists because the blank-push endpoint takes its devices as an array:
// one invocation pushes to every listed device in a single request. The two
// selectors are additive rather than exclusive, so a configuration may mix known
// management ids with serial numbers that need resolving.
func targetListAttributes(deviceNoun string) map[string]actionschema.Attribute {
	return map[string]actionschema.Attribute{
		"management_ids": actionschema.ListAttribute{
			Optional:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Jamf Pro Management IDs of the " + deviceNoun + "s to target. These are the `id` values reported by the `jamfplatform_devices`/`jamfplatform_device` data sources. All listed " + deviceNoun + "s are commanded in a single request. Set this and/or `serial_numbers`.",
			Validators: []validator.List{
				listvalidator.SizeAtLeast(1),
				listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
			},
		},
		"serial_numbers": actionschema.ListAttribute{
			Optional:            true,
			ElementType:         types.StringType,
			MarkdownDescription: "Serial numbers of the " + deviceNoun + "s to target (case-sensitive). Each is looked up to find its Management ID before the command is sent, so `management_ids` avoids that lookup. Set this and/or `management_ids`.",
			Validators: []validator.List{
				listvalidator.SizeAtLeast(1),
				listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
			},
		},
	}
}

// deviceTargetListConfigValidators is the plan-time counterpart to
// targetListAttributes: at least one of the two lists must select a device, so
// that supplying NO identifier fails at plan time instead of part-way through
// the apply. At-least-one rather than exactly-one, because the two lists are
// additive and may be combined.
func deviceTargetListConfigValidators() []action.ConfigValidator {
	return []action.ConfigValidator{
		actionvalidator.AtLeastOneOf(
			path.MatchRoot("management_ids"),
			path.MatchRoot("serial_numbers"),
		),
	}
}
