// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package mdmactions holds the Jamf Pro MDM command actions: send_blank_push,
// renew_mdm_profile, flush_mdm_commands, and one action per command sent through
// POST /pro/v2/mdm/commands.
//
// Fourteen of those per-command actions were removed at the Platform API GA
// because the endpoint was withdrawn (commit 3825dbe7), and the endpoint was
// routed again at Jamf Pro v2508 (SDK v1.9.0, wire-probed 2026-10-22). They are
// being rebuilt in tranches, each command keeping its own action rather than a
// generic one taking a command type: a command's parameters, secrets and
// batchability are its own, and so is the one capability the gateway checks for
// it. The gateway picks that capability from the body, which the SDK registry can
// only express as "any of four", so each action narrows the registry with
// permissions.Narrow to the single one it can need.
//
// Built so far: enable_lost_mode, disable_lost_mode, play_lost_mode_sound.
//
// Three wire facts shape every one of them, all probed on 2026-10-22. The server
// does not check the device platform (a Mac accepts a lost-mode command and the
// command can only fail on the device). It deduplicates by command type per
// device: re-sending one while the first is pending answers 201 with the FIRST
// command's id and discards the new settings, so a changed message sent to an
// offline device silently changes nothing (batchNote says so to the user). And an
// unknown management id fails the whole batch with a 404 that names no device.
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

// batchNote is appended to the description of every command action that targets
// devices in bulk, so the single-request behaviour and its all-or-nothing failure
// mode are visible in the rendered docs rather than buried in this package.
const batchNote = "\n\n" +
	"The action commands every targeted device in a single request. If Jamf Pro cannot match one " +
	"device, the whole request fails, no device is commanded and the error does not say which " +
	"device it was, so a large batch is harder to diagnose than several small ones. Jamf Pro " +
	"documents no maximum batch size."

// batchWarnThreshold is the batch size above which sendCommandBatch warns.
//
// POST /v2/mdm/commands takes clientData as an array and Jamf Pro documents no
// maximum for it: the shipped spec carries no maxItems, and wire probes were
// accepted at 5,000 entries (the sibling blank-push endpoint was accepted at
// 10,000). Where a real cap does exist Jamf documents it — GET /v1/mdm/commands
// states "Limited to 40 UUIDs" and defines a 414 for breaching it — so the
// absence here is meaningful rather than an omission.
//
// The threshold is therefore not a limit but a diagnosability warning: the POST
// is all-or-nothing, and an unresolvable managementId fails the entire request
// with an error that names no id. The larger the batch, the harder that is to
// attribute.
const batchWarnThreshold = 500

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

// sendCommandBatch queues one command for every management id in a single
// POST /v2/mdm/commands.
//
// Only a command whose payload is the same for every device may go through here:
// commandData is shared across the whole clientData array, so a per-device field
// (an unlock token, a local-admin guid) would be applied to all of them. Such a
// command takes one id per invocation instead.
//
// The request is all-or-nothing — see batchWarnThreshold.
func (a *mdmAction) sendCommandBatch(ctx context.Context, resp *action.InvokeResponse, managementIDs []string, commandData *pro.MDMCommandRequestCommandData, label string) bool {
	if len(managementIDs) > batchWarnThreshold {
		resp.Diagnostics.AddWarning(
			"Large Command Batch",
			fmt.Sprintf(
				"%s targets %d devices in a single request. Jamf Pro documents no maximum, but the request is all-or-nothing: if any one device cannot be resolved the whole batch fails with an error that does not identify which. Consider splitting this into smaller invocations.",
				label, len(managementIDs),
			),
		)
	}

	clients := buildClientData(managementIDs)

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Requesting %s for %d device(s)", label, len(managementIDs))})

	commands, err := a.client.SendMdmCommandV2(ctx, &pro.MDMCommandRequest{ClientData: &clients, CommandData: commandData})
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("%s Failed", label),
			fmt.Sprintf("Unable to queue %s for %d device(s)%s: %s", label, len(managementIDs), batchTargetSuffix(managementIDs), helpers.APIErrorDetail(err)),
		)
		return false
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("%s accepted for %d device(s) (%d command(s) queued)", label, len(managementIDs), len(commands))})
	return true
}

// buildClientData turns management ids into the per-device clientData entries
// the batched command request carries.
//
// Each entry holds a POINTER to its id, so every element must reference a
// distinct string. Getting that wrong would not fail loudly: the request would
// stay well-formed and simply command one device N times while leaving the rest
// untouched. Extracted as a pure function so that contract is directly testable.
func buildClientData(managementIDs []string) []pro.MDMCommandClientRequest {
	clients := make([]pro.MDMCommandClientRequest, 0, len(managementIDs))
	for _, id := range managementIDs {
		clients = append(clients, pro.MDMCommandClientRequest{ManagementID: &id})
	}
	return clients
}

// batchTargetSuffix names the targeted devices in an error message when the
// batch is small enough to be useful. The failure response identifies no
// individual device, so echoing the batch back is the only attribution a user
// gets; beyond a handful of ids it becomes noise rather than help.
func batchTargetSuffix(managementIDs []string) string {
	const maxNamed = 10
	if len(managementIDs) == 0 || len(managementIDs) > maxNamed {
		return ""
	}
	return " (" + strings.Join(managementIDs, ", ") + ")"
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

// resolveSerialsForManagementIDs maps Management IDs to serial numbers through
// the Platform devices inventory, for a command that addresses devices by serial
// number. The id is an RSQL-filterable field (wire-probed 2026-10-22), so N ids
// resolve in ceil(N/chunk) requests, chunked by encoded size like
// resolveSerialNumbers. Every id that does not resolve is named.
func (a *mdmAction) resolveSerialsForManagementIDs(ctx context.Context, resp *action.InvokeResponse, ids []string) ([]string, bool) {
	chunks := chunkSerialsByEncodedSize(ids, serialFilterBudgetBytes)

	found := make(map[string]string, len(ids))
	for i, chunk := range chunks {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf(
			"Resolving %d Management ID(s) (request %d of %d)", len(chunk), i+1, len(chunks))})

		quoted := make([]string, 0, len(chunk))
		for _, id := range chunk {
			quoted = append(quoted, quoteRSQLValue(id))
		}

		devices, err := a.devices.ListDevices(ctx, nil, "id=in=("+strings.Join(quoted, ",")+")")
		if err != nil {
			resp.Diagnostics.AddError(
				"Device Lookup Failed",
				fmt.Sprintf("Unable to resolve %d Management ID(s) to serial numbers: %s", len(chunk), helpers.APIErrorDetail(err)),
			)
			return nil, false
		}
		for _, device := range devices {
			found[device.ID] = device.SerialNumber
		}
	}

	serials := make([]string, 0, len(ids))
	var missing []string
	for _, id := range ids {
		serial, ok := found[id]
		if !ok || serial == "" {
			missing = append(missing, id)
			continue
		}
		serials = append(serials, serial)
	}
	if len(missing) > 0 {
		resp.Diagnostics.AddError(
			"Device Not Found",
			fmt.Sprintf("No device with a serial number matched the Management ID(s) %s.", strings.Join(missing, ", ")),
		)
		return nil, false
	}
	return serials, true
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
// selector used by the command actions that target devices in bulk. deviceNoun
// tunes the description (e.g. "computer", "mobile device").
//
// These are lists because the command takes its devices as an array: one
// invocation commands every listed device in a single request. The two selectors
// are additive rather than exclusive, so a configuration may mix known
// management ids with serial numbers that need resolving.
func targetListAttributes(deviceNoun string) map[string]actionschema.Attribute {
	return selectorAttributes(deviceNoun,
		"Each is looked up to find its Management ID before the command is sent, so `management_ids` avoids that lookup.",
		"")
}

// selectorAttributes builds the two selectors with the lookup sentence that is
// true for the action: most commands need a Management ID and resolve serials to
// one, while the wallpaper command takes serials as they are and resolves
// Management IDs the other way round.
func selectorAttributes(deviceNoun, serialLookupNote, managementLookupNote string) map[string]actionschema.Attribute {
	managementDesc := "Jamf Pro Management IDs of the " + deviceNoun + "s to target. These are the `id` values reported by the `jamfplatform_devices`/`jamfplatform_device` data sources. The action commands all listed " + deviceNoun + "s in a single request."
	if managementLookupNote != "" {
		managementDesc += " " + managementLookupNote
	}
	managementDesc += " Set this and/or `serial_numbers`."

	serialDesc := "Serial numbers of the " + deviceNoun + "s to target (case-sensitive)."
	if serialLookupNote != "" {
		serialDesc += " " + serialLookupNote
	}
	serialDesc += " Set this and/or `management_ids`."

	return map[string]actionschema.Attribute{
		"management_ids": actionschema.ListAttribute{
			Optional:            true,
			ElementType:         types.StringType,
			MarkdownDescription: managementDesc,
			Validators: []validator.List{
				listvalidator.SizeAtLeast(1),
				listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
			},
		},
		"serial_numbers": actionschema.ListAttribute{
			Optional:            true,
			ElementType:         types.StringType,
			MarkdownDescription: serialDesc,
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
