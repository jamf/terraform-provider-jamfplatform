// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// deferralUntilUtcPattern matches the classic /policies wire form for
// <allow_deferral_until_utc>: ISO-8601 with millisecond precision and a
// four-digit numeric offset, e.g. "2027-01-01T01:00:00.000+0000". The
// classic API rejects any other format with HTTP 409.
var deferralUntilUtcPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}[+-]\d{4}$`)

// activationExpirationDatePattern matches the classic /policies wire form
// for <activation_date> and <expiration_date>: 24-hour `YYYY-MM-DD HH:MM:SS`
// with a single space separator (e.g. "2027-06-01 14:30:00"). Wire-probed
// 2026-05-27 against /JSSResource/policies/7029: send `5:00 PM` style and
// the value is silently dropped; send `17:00` and the server returns HTTP
// 409 ("Problem with date_time_limitations"). The companion epoch / UTC
// echoes (`*_epoch`, `*_utc`) are derived server-side and not modelled.
var activationExpirationDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)

// noExecuteTimePattern matches the classic /policies wire form for
// <no_execute_start> and <no_execute_end>: 12-hour `h:MM AM` / `h:MM PM`
// with the hour 1-12 (no leading zero) and a literal space before the
// meridiem (e.g. "1:00 AM", "12:30 PM"). It is what the wire *reads back*,
// and it is a subset of what the write path parses — 24-hour HH:MM, a
// non-breaking or doubled space, and every punctuated meridiem answer HTTP
// 409, while `1:00 am` and `01:00 AM` are accepted.
//
// This is the shape a practitioner writes and the shape the GET returns. It is
// NOT the shape sent on the wire — see no_execute.go, which offsets the value
// 48 hours forward because that is the only form Jamf Pro stores.
var noExecuteTimePattern = regexp.MustCompile(`^(1[0-2]|[1-9]):[0-5]\d (AM|PM)$`)

// deferralTypeCompanionsValidator enforces the cross-field shape of the
// `user_interaction.deferral_type` enum against its type-specific siblings
// `deferral_until_utc` and `deferral_days`:
//
//   - `deferral_type = "none"`     forbids both siblings.
//   - `deferral_type = "date"`     requires `deferral_until_utc`, forbids `deferral_days`.
//   - `deferral_type = "duration"` requires `deferral_days` (>=1), forbids `deferral_until_utc`.
//
// The rule is value-discriminated (different value of `deferral_type`
// implies a different companion set), so per STYLE_GUIDE.md §Cross-field
// validation a custom `validator.String` is the right tool — off-the-shelf
// `AlsoRequires` / `ConflictsWith` fire on any value and cannot express
// "required only when this string equals X". Errors attach to the
// companion's path so the user looks at the field they need to fix.
type deferralTypeCompanionsValidator struct{}

// DeferralTypeCompanionsValidator constructs the validator.
func DeferralTypeCompanionsValidator() validator.String {
	return deferralTypeCompanionsValidator{}
}

// Description returns a plain-text description of the validator.
func (deferralTypeCompanionsValidator) Description(_ context.Context) string {
	return "user_interaction.deferral_type controls which of deferral_until_utc / deferral_days must be present"
}

// MarkdownDescription returns the markdown description.
func (v deferralTypeCompanionsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (deferralTypeCompanionsValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	untilPath := req.Path.ParentPath().AtName("deferral_until_utc")
	daysPath := req.Path.ParentPath().AtName("deferral_days")

	var untilVal types.String
	var daysVal types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, untilPath, &untilVal)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, daysPath, &daysVal)...)
	if resp.Diagnostics.HasError() {
		return
	}

	untilSet := !untilVal.IsNull() && !untilVal.IsUnknown()
	daysSet := !daysVal.IsNull() && !daysVal.IsUnknown()

	// When deferral_type is UNKNOWN (e.g. sourced from a variable) a
	// config-time validator must DEFER — see STYLE_GUIDE "Config-time
	// validators MUST defer on unknown values". The discriminator's value is
	// not yet decidable, so we cannot judge the companions.
	if req.ConfigValue.IsUnknown() {
		return
	}

	// When deferral_type is genuinely NULL we still reject orphaned siblings —
	// the trio is a single Optional concept and a companion without its
	// discriminator is meaningless. These are forbidden-when checks (error on
	// PRESENT), so unknown companions are correctly ignored (untilSet/daysSet
	// are false for unknown).
	if req.ConfigValue.IsNull() {
		if untilSet {
			resp.Diagnostics.AddAttributeError(
				untilPath,
				"deferral_until_utc requires deferral_type = \"date\"",
				"Set `deferral_type = \"date\"` to use a deferral cut-off, or remove `deferral_until_utc`.",
			)
		}
		if daysSet {
			resp.Diagnostics.AddAttributeError(
				daysPath,
				"deferral_days requires deferral_type = \"duration\"",
				"Set `deferral_type = \"duration\"` to use a deferral duration, or remove `deferral_days`.",
			)
		}
		return
	}

	switch req.ConfigValue.ValueString() {
	case "none":
		if untilSet {
			resp.Diagnostics.AddAttributeError(
				untilPath,
				"deferral_until_utc forbidden when deferral_type = \"none\"",
				"Remove `deferral_until_utc`, or set `deferral_type = \"date\"`.",
			)
		}
		if daysSet {
			resp.Diagnostics.AddAttributeError(
				daysPath,
				"deferral_days forbidden when deferral_type = \"none\"",
				"Remove `deferral_days`, or set `deferral_type = \"duration\"`.",
			)
		}
	case "date":
		// Required-when check: error only when deferral_until_utc is genuinely
		// null; defer when it is unknown (unknown != absent).
		if untilVal.IsNull() {
			resp.Diagnostics.AddAttributeError(
				untilPath,
				"deferral_until_utc required when deferral_type = \"date\"",
				"Provide a UTC ISO-8601 cut-off (e.g. `2027-01-01T01:00:00.000+0000`).",
			)
		}
		if daysSet {
			resp.Diagnostics.AddAttributeError(
				daysPath,
				"deferral_days forbidden when deferral_type = \"date\"",
				"Remove `deferral_days`, or set `deferral_type = \"duration\"`.",
			)
		}
	case "duration":
		// Required-when check: error only when deferral_days is genuinely null;
		// defer when it is unknown (unknown != absent).
		if daysVal.IsNull() {
			resp.Diagnostics.AddAttributeError(
				daysPath,
				"deferral_days required when deferral_type = \"duration\"",
				"Provide a positive day count (e.g. `deferral_days = 3`).",
			)
		} else if !daysVal.IsUnknown() && daysVal.ValueInt64() < 1 {
			resp.Diagnostics.AddAttributeError(
				daysPath,
				"deferral_days must be >= 1",
				fmt.Sprintf("Got %d. The classic API stores this as minutes — the provider multiplies by 1440.", daysVal.ValueInt64()),
			)
		}
		if untilSet {
			resp.Diagnostics.AddAttributeError(
				untilPath,
				"deferral_until_utc forbidden when deferral_type = \"duration\"",
				"Remove `deferral_until_utc`, or set `deferral_type = \"date\"`.",
			)
		}
	}
}

// frequencyOncePerComputer is the one general.frequency under which Jamf Pro
// keeps a policy's retry configuration. Taken from the SDK's own enum rather
// than restated, so a spec ingest that renames it fails the build.
const frequencyOncePerComputer = proclassic.PolicyPostGeneralFrequencyOncePerComputer

// retryRequiresOncePerComputerValidator refuses a retry configuration on a
// policy whose frequency cannot carry one.
//
// Jamf Pro persists general.retry_event, general.retry_attempts and
// general.notify_on_each_failed_retry only while general.frequency is
// "Once per computer". Under any other frequency it accepts the write with
// HTTP 201 and silently resets the three to `none`, `-1` and `false` —
// wire-probed against 11.31.1 on 2026-09-07, and independent of the triggers
// (a policy with trigger_checkin = false kept its retry configuration).
//
// Without this check the failure lands mid-apply as "Provider produced
// inconsistent result after apply", and it lands on a config that never
// mentioned the offending attribute: retry_* and notify_* are
// Optional+Computed, so a value set under an earlier "Once per computer" is
// carried into the plan by UseNonNullStateForUnknown when the practitioner
// changes nothing but the frequency.
type retryRequiresOncePerComputerValidator struct{}

// Description returns a plain-text description of the validator.
func (retryRequiresOncePerComputerValidator) Description(context.Context) string {
	return `general.retry_event, general.retry_attempts and general.notify_on_each_failed_retry require general.frequency = "` + frequencyOncePerComputer + `"`
}

// MarkdownDescription returns the markdown description.
func (v retryRequiresOncePerComputerValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time cross-field check.
//
// It reads one attribute at a time rather than decoding the whole model. A
// policy configuration routinely carries unknown nested values (an interpolated
// scope id, a package reference), and Config.Get on the whole model fails
// outright on those, which would disable every validator on the resource.
func (retryRequiresOncePerComputerValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var frequency types.String
	if diags := req.Config.GetAttribute(ctx, path.Root("general").AtName("frequency"), &frequency); diags.HasError() {
		return
	}
	if frequency.IsNull() || frequency.IsUnknown() || frequency.ValueString() == frequencyOncePerComputer {
		return
	}

	var retryEvent types.String
	if diags := req.Config.GetAttribute(ctx, path.Root("general").AtName("retry_event"), &retryEvent); !diags.HasError() {
		if isStringSet(retryEvent) && retryEvent.ValueString() != retryEventNone {
			addRetryFrequencyError(resp, "retry_event", frequency.ValueString(), `"`+retryEvent.ValueString()+`"`, `"`+retryEventNone+`"`)
		}
	}

	var retryAttempts types.Int64
	if diags := req.Config.GetAttribute(ctx, path.Root("general").AtName("retry_attempts"), &retryAttempts); !diags.HasError() {
		if !retryAttempts.IsNull() && !retryAttempts.IsUnknown() && retryAttempts.ValueInt64() != retryAttemptsNone {
			addRetryFrequencyError(resp, "retry_attempts", frequency.ValueString(), strconv.FormatInt(retryAttempts.ValueInt64(), 10), "-1")
		}
	}

	var notify types.Bool
	if diags := req.Config.GetAttribute(ctx, path.Root("general").AtName("notify_on_each_failed_retry"), &notify); !diags.HasError() {
		if !notify.IsNull() && !notify.IsUnknown() && notify.ValueBool() {
			addRetryFrequencyError(resp, "notify_on_each_failed_retry", frequency.ValueString(), "true", "false")
		}
	}
}

// retryEventNone and retryAttemptsNone are the values Jamf Pro resets a retry
// configuration to when the frequency cannot carry one — which makes them the
// only values the three attributes may hold under such a frequency.
const (
	retryEventNone    = proclassic.PolicyPostGeneralRetryEventNone
	retryAttemptsNone = -1
)

// addRetryFrequencyError reports one retry attribute set against an
// incompatible frequency, naming the value Jamf Pro would silently store.
func addRetryFrequencyError(resp *resource.ValidateConfigResponse, attr, frequency, got, allowed string) {
	resp.Diagnostics.AddAttributeError(
		path.Root("general").AtName(attr),
		fmt.Sprintf("general.%s requires frequency %q", attr, frequencyOncePerComputer),
		fmt.Sprintf(
			"general.frequency is %q, and Jamf Pro keeps a policy's retry configuration only under %q. It accepts the change and then resets general.%s to %s without reporting an error, "+
				"which Terraform reports as \"Provider produced inconsistent result after apply\".\n\n"+
				"Set general.frequency = %q, or set general.%s = %s.\n\n"+
				"general.%s is Optional+Computed, so a value an earlier apply set under %q carries into this plan even when the configuration no longer mentions it. "+
				"You may need to set %s here to clear it.",
			frequency, frequencyOncePerComputer, attr, allowed,
			frequencyOncePerComputer, attr, allowed,
			attr, frequencyOncePerComputer, allowed,
		),
	)
}

// isStringSet reports whether a string attribute carries a usable (known,
// non-null, non-empty) value at config time.
func isStringSet(s types.String) bool {
	return !s.IsNull() && !s.IsUnknown() && s.ValueString() != ""
}

// createAccountRequiresHomeValidator enforces local_accounts[].home whenever the
// entry's action is "Create".
//
// Jamf Pro refuses a create-account entry with no home directory — wire-probed
// 2026-09-08 against /JSSResource/policies/id/0, which answers `409 Problem with
// create account fields` and names nothing. Adding `home` alone turns the same
// request into a 201, with username, realname, password and admin unchanged. The
// message is identical whatever else the entry carries, so a plan-time check is
// the only way the practitioner learns which field is missing.
//
// An EMPTY home is refused on the same terms. The same probe sent
// `<home></home>` and was refused identically to sending no `home` at all, while
// `<home>/Users/...</home>` returned 201 and round-tripped. So a stored blank
// home cannot exist by way of the API, and the check treats empty and absent as
// one case.
//
// The check reads CONFIGURATION, not state, and that is deliberate rather than
// an oversight. `home` is Optional+Computed with UseNonNullStateForUnknown, so a
// configuration that applied once with `home` set and later drops the attribute
// is legal today: the plan modifier carries the prior value into the plan and
// the update sends it. This validator refuses that configuration too, which is
// stronger than what Jamf Pro alone requires — and the schema description says
// so. The alternative does not exist: on a create the planned value of an
// Optional+Computed attribute is unknown, so a ModifyPlan variant reading the
// plan could never fire on the one apply where the 409 actually lands. A check
// that is silent exactly where it is needed is worse than one that asks for the
// attribute to stay in the configuration.
type createAccountRequiresHomeValidator struct{}

// Description returns a plain-text description of the validator.
func (createAccountRequiresHomeValidator) Description(context.Context) string {
	return `local_accounts entries with action = "Create" require home`
}

// MarkdownDescription returns the markdown description.
func (v createAccountRequiresHomeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time check.
//
// As on retryRequiresOncePerComputerValidator, attributes are read one at a time:
// a policy configuration routinely carries unknown nested values, and Config.Get
// over the whole model fails outright on those, disabling every validator here.
func (createAccountRequiresHomeValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var accounts types.List
	if diags := req.Config.GetAttribute(ctx, path.Root("local_accounts"), &accounts); diags.HasError() {
		return
	}
	if accounts.IsNull() || accounts.IsUnknown() {
		return
	}

	for i := range accounts.Elements() {
		element := path.Root("local_accounts").AtListIndex(i)

		var action types.String
		if diags := req.Config.GetAttribute(ctx, element.AtName("action"), &action); diags.HasError() {
			continue
		}
		if !isStringSet(action) || action.ValueString() != accountActionCreate {
			continue
		}

		var home types.String
		if diags := req.Config.GetAttribute(ctx, element.AtName("home"), &home); diags.HasError() {
			continue
		}
		if home.IsUnknown() || isStringSet(home) {
			continue
		}

		resp.Diagnostics.AddAttributeError(
			element.AtName("home"),
			"home required for a Create account action",
			`This local_accounts entry has action = "Create", which Jamf Pro will not accept without `+
				"`home`. It answers `409 Problem with create account fields` and names no field. Set "+
				"`home` to the account's home directory path (e.g. \"/Users/<username>\"). This check reads "+
				"your configuration rather than Terraform state, so `home` has to stay in a `Create` entry "+
				"even on a policy that already applied with it.",
		)
	}
}

// accountActionCreate is the local_accounts action Jamf Pro gates on `home`,
// aliased from the SDK enum so the two cannot drift.
const accountActionCreate = string(proclassic.PolicyAccountMaintenanceAccountsAccountItemActionCreate)

// diskEncryptionCompanionsValidator checks the disk_encryption block's fields
// against the action that gives them meaning.
//
// Jamf Pro treats the block as a discriminated union on `action`, wire-probed
// 2026-10-13 against /JSSResource/policies/id/0:
//
//   - `apply` needs disk_encryption_configuration_id, positive. Without it,
//     with 0 or with -1, the answer is `409 Disk encryption id not provided`.
//   - `remediate` with remediate_key_type `Institutional` or `Individual And
//     Institutional` needs remediate_disk_encryption_configuration_id, positive:
//     `409 Remediate disk encryption id not provided`. With `Individual`, or no
//     key type (which reads back as `Individual`), the id is dropped.
//   - disk_encryption_configuration_id and auth_restart are dropped unless the
//     action is `apply`, and remediate_key_type and the remediate id unless it
//     is `remediate`. The write returns 201 and the field is simply absent from
//     the read, so a configuration naming one believes it took effect.
//
// The check reads CONFIGURATION and acts only when `action` is configured and
// known: an omitted action may be carried from state, and a configuration that
// names a companion without an action is not provably wrong on an update. The
// same trade as createAccountRequiresHomeValidator, which describes it in full.
type diskEncryptionCompanionsValidator struct{}

// Description returns a plain-text description of the validator.
func (diskEncryptionCompanionsValidator) Description(context.Context) string {
	return "disk_encryption fields must match disk_encryption.action"
}

// MarkdownDescription returns the markdown description.
func (v diskEncryptionCompanionsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time check. Attributes are read one at a
// time; see retryRequiresOncePerComputerValidator for why.
func (diskEncryptionCompanionsValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	root := path.Root("disk_encryption")
	var action types.String
	if req.Config.GetAttribute(ctx, root.AtName("action"), &action).HasError() || !isStringSet(action) {
		return
	}

	var (
		configID, remediateID types.Int64
		authRestart           types.Bool
		keyType               types.String
	)
	for name, target := range map[string]any{
		"disk_encryption_configuration_id":           &configID,
		"remediate_disk_encryption_configuration_id": &remediateID,
		"auth_restart":       &authRestart,
		"remediate_key_type": &keyType,
	} {
		if req.Config.GetAttribute(ctx, root.AtName(name), target).HasError() {
			return
		}
	}

	absent := func(v types.Int64) bool { return v.IsNull() || (!v.IsUnknown() && v.ValueInt64() < 1) }
	ignored := func(attr, needs string) {
		resp.Diagnostics.AddAttributeError(root.AtName(attr), attr+" is ignored for this action",
			"Jamf Pro discards `"+attr+"` unless `disk_encryption.action` is "+needs+". The setting would never apply. "+
				"Remove it or change the action.")
	}

	switch action.ValueString() {
	case string(proclassic.PolicyPostDiskEncryptionActionApply):
		if absent(configID) {
			resp.Diagnostics.AddAttributeError(root.AtName("disk_encryption_configuration_id"),
				"disk_encryption_configuration_id required for the apply action",
				"Jamf Pro refuses an `apply` with no configuration ID, or one below 1, and names no field. "+
					"Set `disk_encryption_configuration_id` to the ID of a disk encryption configuration.")
		}
		if !keyType.IsNull() {
			ignored("remediate_key_type", "`remediate`")
		}
		if !remediateID.IsNull() {
			ignored("remediate_disk_encryption_configuration_id", "`remediate`")
		}
	case string(proclassic.PolicyPostDiskEncryptionActionRemediate):
		if !configID.IsNull() {
			ignored("disk_encryption_configuration_id", "`apply`")
		}
		if !authRestart.IsNull() && !authRestart.IsUnknown() && authRestart.ValueBool() {
			ignored("auth_restart", "`apply`")
		}
		institutional := !keyType.IsNull() && !keyType.IsUnknown() && keyType.ValueString() != string(proclassic.PolicyDiskEncryptionRemediateKeyTypeIndividual)
		switch {
		case institutional && absent(remediateID):
			resp.Diagnostics.AddAttributeError(root.AtName("remediate_disk_encryption_configuration_id"),
				"remediate_disk_encryption_configuration_id required for this key type",
				"Jamf Pro refuses a `remediate` with key type `"+keyType.ValueString()+
					"` and no configuration ID, or one below 1. Set `remediate_disk_encryption_configuration_id`.")
		case !institutional && !keyType.IsUnknown() && !remediateID.IsNull():
			ignored("remediate_disk_encryption_configuration_id", "`remediate` with remediate_key_type `Institutional` or `Individual And Institutional`")
		}
	default:
		if !configID.IsNull() {
			ignored("disk_encryption_configuration_id", "`apply`")
		}
		if !authRestart.IsNull() && !authRestart.IsUnknown() && authRestart.ValueBool() {
			ignored("auth_restart", "`apply`")
		}
		if !keyType.IsNull() {
			ignored("remediate_key_type", "`remediate`")
		}
		if !remediateID.IsNull() {
			ignored("remediate_disk_encryption_configuration_id", "`remediate`")
		}
	}
}

// validateEFIPasswordOnCreate refuses efi_password.of_mode = "command" on a
// create when no password is configured.
//
// Jamf Pro accepts that write and stores a policy whose EFI password hash is
// empty, wire-probed 2026-10-13: the payload reads back `of_mode: command` with a
// blank `of_password_sha256`, which protects nothing while looking enforced.
//
// It runs on create only. `of_password` is WriteOnly, so a later update that
// leaves it out is how the provider is told to retain the stored password (see
// of_password_wo_version), and the config alone cannot tell that apart from a
// create that forgot it.
func validateEFIPasswordOnCreate(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if !req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	root := path.Root("efi_password")
	var mode types.String
	if req.Config.GetAttribute(ctx, root.AtName("of_mode"), &mode).HasError() || !isStringSet(mode) ||
		mode.ValueString() != string(proclassic.PolicyAccountMaintenanceOpenFirmwareEfiPasswordOfModeCommand) {
		return
	}
	var password types.String
	if req.Config.GetAttribute(ctx, root.AtName("of_password"), &password).HasError() || password.IsUnknown() || isStringSet(password) {
		return
	}
	resp.Diagnostics.AddAttributeError(root.AtName("of_password"),
		"of_password required for the command mode",
		"Jamf Pro accepts `of_mode = \"command\"` without a password and stores an empty hash. The policy would "+
			"report an EFI password it does not enforce. Set `of_password` and `of_password_wo_version = 1`.")
}

// retryAttemptsMin and retryAttemptsMax bound general.retry_attempts while a
// retry event is set: Jamf Pro answers `409 Retry attempts value should be
// between 1 and 10` outside them, -1 included. Wire-probed 2026-10-13.
const (
	retryAttemptsMin = 1
	retryAttemptsMax = 10
)

// retryAttemptsMatchEventValidator checks general.retry_attempts against the
// retry event that gives it meaning.
//
// Wire-probed 2026-10-13: under retry_event `none` Jamf Pro answers 201 for any
// retry_attempts and stores -1, so a configured 5 reads back as -1 and the apply
// ends in "Provider produced inconsistent result". Under `check-in` or `trigger`
// the value must be 1 to 10, and -1 is refused with a 409 that says so.
//
// It acts only when both are configured and known. An omitted retry_event may
// be carried from state, so a lone retry_attempts is not provably wrong; a create
// that sets an event and no attempts is caught by validateRetryAttemptsOnCreate.
type retryAttemptsMatchEventValidator struct{}

// Description returns a plain-text description of the validator.
func (retryAttemptsMatchEventValidator) Description(context.Context) string {
	return "general.retry_attempts must match general.retry_event"
}

// MarkdownDescription returns the markdown description.
func (v retryAttemptsMatchEventValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time cross-field check.
func (retryAttemptsMatchEventValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	general := path.Root("general")
	var event types.String
	var attempts types.Int64
	if req.Config.GetAttribute(ctx, general.AtName("retry_event"), &event).HasError() ||
		req.Config.GetAttribute(ctx, general.AtName("retry_attempts"), &attempts).HasError() ||
		!isStringSet(event) || attempts.IsNull() || attempts.IsUnknown() {
		return
	}
	got := attempts.ValueInt64()
	switch {
	case event.ValueString() == retryEventNone && got != retryAttemptsNone:
		resp.Diagnostics.AddAttributeError(general.AtName("retry_attempts"),
			"general.retry_attempts is ignored when retry_event is none",
			fmt.Sprintf("Jamf Pro stores -1 for retry_attempts while general.retry_event is %q, so %d would read back as -1 and fail the apply. "+
				"Set general.retry_event to %q or %q, or set general.retry_attempts = -1.",
				retryEventNone, got, proclassic.PolicyPostGeneralRetryEventCheckIn, proclassic.PolicyPostGeneralRetryEventTrigger))
	case event.ValueString() != retryEventNone && (got < retryAttemptsMin || got > retryAttemptsMax):
		resp.Diagnostics.AddAttributeError(general.AtName("retry_attempts"),
			"general.retry_attempts must be 1 to 10 when retry_event is set",
			fmt.Sprintf("general.retry_event is %q, and Jamf Pro refuses %d. "+
				"Use a value from 1 to 10, or set general.retry_event = %q.", event.ValueString(), got, retryEventNone))
	}
}

// validateRetryAttemptsOnCreate refuses a create that names a retry event and no
// retry_attempts. Jamf Pro answers `409 Retry attempts value should be between 1
// and 10` and names no attribute, wire-probed 2026-10-13. On an update the value
// may legitimately be carried from state, so only a create is checked.
func validateRetryAttemptsOnCreate(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if !req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	general := path.Root("general")
	var event types.String
	var attempts types.Int64
	if req.Config.GetAttribute(ctx, general.AtName("retry_event"), &event).HasError() ||
		req.Config.GetAttribute(ctx, general.AtName("retry_attempts"), &attempts).HasError() ||
		!isStringSet(event) || event.ValueString() == retryEventNone || !attempts.IsNull() {
		return
	}
	resp.Diagnostics.AddAttributeError(general.AtName("retry_attempts"),
		"general.retry_attempts required when retry_event is set",
		fmt.Sprintf("general.retry_event is %q, and Jamf Pro refuses the policy when no retry_attempts is given. "+
			"Set general.retry_attempts to a value from 1 to 10.", event.ValueString()))
}

// deferralUntilLayout is the wire form of user_interaction.deferral_until_utc,
// the format deferralUntilUtcPattern matches.
const deferralUntilLayout = "2006-01-02T15:04:05.000-0700"

// validateDeferralDateIsFuture refuses a new or changed
// user_interaction.deferral_until_utc that is already in the past.
//
// Jamf Pro refuses such a date with `409 allow_deferral_until_utc must be a date
// in the future`, wire-probed 2026-10-13. Only a date this plan changes is
// checked. A policy created before the date passed still holds it, and an update
// that leaves it alone is legal: omitUnchangedDeferralDate keeps it off the wire.
// Flagging it would fail every plan on that policy until someone edited the date.
//
// now is a parameter so the test does not depend on the clock.
func validateDeferralDateIsFuture(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse, now time.Time) {
	if req.Plan.Raw.IsNull() {
		return
	}
	attr := path.Root("user_interaction").AtName("deferral_until_utc")
	var planned, prior types.String
	if req.Plan.GetAttribute(ctx, attr, &planned).HasError() || !isStringSet(planned) {
		return
	}
	if !req.State.Raw.IsNull() {
		if req.State.GetAttribute(ctx, attr, &prior).HasError() {
			return
		}
		if prior.Equal(planned) {
			return
		}
	}
	until, err := time.Parse(deferralUntilLayout, planned.ValueString())
	if err != nil || until.After(now) {
		return
	}
	resp.Diagnostics.AddAttributeError(attr,
		"user_interaction.deferral_until_utc is in the past",
		"Jamf Pro refuses a deferral date that has already passed. Set a future date. "+
			"A policy that already holds a date keeps working after it passes, provided you leave the date unchanged.")
}
