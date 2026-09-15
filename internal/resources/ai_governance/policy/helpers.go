// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// isNotFound reports whether an error is the platform saying the policy is gone. A malformed
// identifier and an already-archived policy both answer with this code, so read, update, publish and
// delete all treat it the same way.
func isNotFound(err error) bool {
	return hasCode(err, codePolicyNotFound)
}

// isVersionConflict reports whether an error is the platform refusing an update because the policy
// has moved on since the version the request named. Only update can produce it, so it is checked
// there rather than in appendWriteDiagnostics, which create shares.
func isVersionConflict(err error) bool {
	return hasCode(err, codePolicyVersionConflict)
}

// hasCode reports whether an error carries the given machine-readable code.
func hasCode(err error, code string) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	for _, detail := range apiErr.Details() {
		if detail.Code == code {
			return true
		}
	}
	return false
}

// appendWriteDiagnostics turns a create or update failure into the most specific diagnostic the
// error body supports, and reports whether it recognised one.
//
// Only codes whose remedy is not obvious from the platform's own wording are translated, and each
// one is pointed at the attribute the operator actually wrote — the platform names wire fields
// (`toolId`, `settings`), which a practitioner reading a Terraform diagnostic has never typed.
//
// SCHEMA_VALIDATION_FAILED earns the most work because it is the code plan-time validation is meant
// to pre-empt: reaching it at apply means either the settings changed shape after the plan, or the
// schema declares something the provider's own checker skips. Its `field` is a JSON pointer into the
// settings when the platform can locate the value and empty when the problem is the object itself,
// so the pointer is quoted into the detail rather than used to build an attribute path.
//
// REQUEST_CONTEXT_NOT_PROVIDED is deliberately absent — see mappings.go for why.
func appendWriteDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}

	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeToolIDUnknown:
			diags.AddAttributeError(
				path.Root("tool_id"),
				"Unknown AI tool",
				"The platform offers no AI tool with this identifier. Read the available identifiers from the "+
					"jamfplatform_ai_governance_tools data source — they are reverse-domain names such as "+
					"com.anthropic.claudecode, and the match is exact. Reported by Jamf: "+detail.Description,
			)
		case codeSchemaVersionUnknown:
			diags.AddAttributeError(
				path.Root("schema_version"),
				"Unknown settings schema version",
				"This tool does not offer the requested settings schema version. Read the versions it does offer from "+
					"the jamfplatform_ai_governance_tool data source's schema_versions attribute. Reported by Jamf: "+
					detail.Description,
			)
		case codeSchemaValidationFailed:
			diags.AddAttributeError(
				path.Root("settings_json"),
				"Settings do not match the tool's schema",
				schemaFailureDetail(detail.Field, detail.Description),
			)
		case codeValidationFailed:
			diags.AddError(
				"Jamf rejected the policy",
				"Jamf rejected this policy on the field "+quoteOrUnnamed(detail.Field)+". Reported by Jamf: "+
					detail.Description,
			)
		default:
			continue
		}
		matched = true
	}
	return matched
}

// schemaFailureDetail composes the detail for a settings validation failure, locating it by the
// platform's own JSON pointer when there is one.
func schemaFailureDetail(field, description string) string {
	location := "The settings were rejected"
	if field != "" {
		location = "The setting at " + field + " was rejected"
	}
	return location + " when checked against this tool's schema for the requested schema_version. " +
		"The provider checks the same schema during plan, so reaching this at apply means either the settings " +
		"changed after the plan was made, or the rule involved is one the provider's own check does not cover. " +
		"Reported by Jamf: " + description
}

// quoteOrUnnamed renders a wire field name for a diagnostic, saying so when the platform named none.
func quoteOrUnnamed(field string) string {
	if strings.TrimSpace(field) == "" {
		return "it did not name"
	}
	return "\"" + field + "\""
}

// joinAlternatives renders a set of names as a regular-expression alternation.
func joinAlternatives(names []string) string {
	return strings.Join(names, "|")
}

// joinCommas renders a set of names for a diagnostic.
func joinCommas(names []string) string {
	return strings.Join(names, ", ")
}

// mustCompile compiles a pattern the package itself builds from a fixed set of names. A failure is a
// programming error rather than anything an operator can cause, and the schema is built before any
// diagnostic sink exists to report it to.
func mustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}

// privateStateReader is the read half of the framework's private-state surface. The concrete type is
// internal to the framework, so a narrow interface is the only way to write a helper against it —
// and it makes the round trip testable without a live resource.
type privateStateReader interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
}

// privateStateWriter is the write half, satisfied uniformly by the create, read and update response
// private-state surfaces.
//
// Every call site assigns the response field to a variable of this type only when the field is
// non-nil, and must keep doing so. The framework's concrete type is unnameable here, so a nil
// pointer assigned straight into the interface would arrive as a non-nil interface holding a nil
// pointer — past the guard below and into SetKey, which reports an uninitialized-ProviderData error
// on a nil receiver. The framework populates the field on every real operation; a response built
// without one is a unit test, which is the reason the narrowing is needed at all, and recording
// nothing is the right outcome there.
type privateStateWriter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// privateState is both halves at once, which is what a test stand-in supplies. A unit test cannot
// populate the framework's own private-state fields — the concrete type is internal and has no
// exported constructor — so PolicyResource carries one field of this type that production leaves
// nil, and without it the whole precondition round trip would be exercised by nothing.
type privateState interface {
	privateStateReader
	privateStateWriter
}

// lockTokenPattern matches the entity-tag forms this provider records and the update endpoint
// documents: a decimal counter, quoted or bare, optionally behind a weak-validator prefix.
//
// `*` is deliberately excluded even though the endpoint accepts it, because the platform reads it as
// "apply regardless of the version" — a private-state value spelled that way would turn the check
// off while the apply reported success, where every other unexpected spelling is refused with
// VALIDATION_FAILED and therefore fails safe.
var lockTokenPattern = mustCompile(`^(?:W/)?(?:"[0-9]+"|[0-9]+)$`)

// privateKeyVersion holds the policy's optimistic-lock counter between one apply and the next, so
// that an update can make its PATCH conditional on nobody else having written the policy since the
// provider last read it.
//
// It lives in private state rather than in the schema for two reasons. The counter is machinery an
// operator can neither set nor usefully read — it increments on every PATCH, including one the
// platform diffs to nothing — so surfacing it would put churn in every plan for a value nothing can
// consume. And adding an attribute to a shipped schema needs a state upgrader, which private state
// does not: a policy recorded before this key existed reads back empty and updates unconditionally,
// exactly as it did before.
const privateKeyVersion = "policy_version"

// readLockToken returns the If-Match precondition recorded by the last read of this policy, or the
// empty string when there is none — a policy the platform created before it gained the counter, one
// imported in this run, or state written before the provider began recording it. The empty string is
// what UpdatePolicy takes to mean "unconditional", so each of those cases degrades to the behaviour
// the resource had before this key existed.
//
// The two failures are reported at different strengths on purpose. A value that is not valid JSON
// carries no version at all, so it is a warning and the update falls back to the unconditional write
// the resource performed before this key existed. A value that decodes but is not a counter is an
// error, because the one spelling the platform singles out is `*`: it would be sent as written,
// accepted, and reported as a successful apply while the check it was meant to make had been turned
// off.
func readLockToken(ctx context.Context, r privateStateReader) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r == nil {
		return "", diags
	}
	raw, d := r.GetKey(ctx, privateKeyVersion)
	diags.Append(d...)
	if diags.HasError() || len(raw) == 0 {
		return "", diags
	}
	var token string
	if err := json.Unmarshal(raw, &token); err != nil {
		diags.AddWarning(
			"Unable to read the AI policy's recorded version",
			"Terraform stored a version for this policy that the provider cannot decode: "+err.Error()+
				". This update goes out without the concurrent-edit check. The next read of the policy records "+
				"a version the check can use.",
		)
		return "", diags
	}
	if !lockTokenPattern.MatchString(token) {
		diags.AddError(
			"Unusable version recorded for the AI policy",
			"Terraform stored "+strconv.Quote(token)+" where this policy's version counter belongs, so the "+
				"provider sent no update. Jamf reads \"*\" as \"apply regardless of the version\", which would "+
				"overwrite whatever the policy now holds and still report success. Run "+
				"\"terraform apply -refresh-only\" to record the version Jamf reports.",
		)
		return "", diags
	}
	return token, diags
}

// writeLockToken records the counter a just-completed read reported, ready for the next update. A
// policy the platform reports without one — every policy that predates the counter — clears the key
// instead, so a value that has gone stale can never be sent as a precondition.
//
// The value is stored already rendered as the header the update will send, as the strong entity-tag
// form RFC 7232 defines for If-Match and the update endpoint's own parameter documentation gives as
// its example. The bare decimal was accepted when the endpoint was probed on 2026-09-09, so the
// quoting guards against a stricter parser rather than a refusal anybody has seen — and a parser
// that ignored an unquoted header rather than refusing it would turn the check off silently, which
// is the failure worth spending two characters on.
//
// One assumption here is unprobed and needs a wire probe before it can be relied on: the counter is
// taken from the policy body's version field, while the endpoint documentation names the ETag
// response header as the source. GetPolicy discards response headers, so the documented source
// cannot be read at all today, and the two have never been compared.
//
// A failure to record the counter is a warning and never an error. It is bookkeeping: losing it
// degrades the next update to the unconditional write the resource performed before this key
// existed, whereas an error here would fire the caller's own error check and leave a policy that
// exists recorded in no state at all — and because policy names are not unique, the next apply would
// then create a second one.
func writeLockToken(ctx context.Context, w privateStateWriter, version *int64) diag.Diagnostics {
	var diags diag.Diagnostics
	if w == nil {
		return diags
	}
	if version == nil {
		appendRecordFailure(&diags, w.SetKey(ctx, privateKeyVersion, nil))
		return diags
	}
	encoded, err := json.Marshal(strconv.Quote(strconv.FormatInt(*version, 10)))
	if err != nil {
		appendLockTokenNotRecorded(&diags)
		return diags
	}
	appendRecordFailure(&diags, w.SetKey(ctx, privateKeyVersion, encoded))
	return diags
}

// appendRecordFailure reports a refused private-state write as one warning of the provider's own,
// and carries any other diagnostic through. It replaces the wording rather than demoting it: the
// framework answers a surface it cannot write with an error prescribing a call to its own
// constructor, which is neither something an operator can act on nor something they should read.
func appendRecordFailure(diags *diag.Diagnostics, from diag.Diagnostics) {
	if from.HasError() {
		appendLockTokenNotRecorded(diags)
	}
	for _, d := range from {
		if d.Severity() != diag.SeverityError {
			diags.Append(d)
		}
	}
}

// appendLockTokenNotRecorded reports that the version Jamf holds for the policy could not be kept
// for the next update, and what that costs.
func appendLockTokenNotRecorded(diags *diag.Diagnostics) {
	diags.AddWarning(
		"Unable to record the AI policy's version",
		"Terraform could not store the version Jamf reports for this policy, so the next update goes out "+
			"unconditionally instead of checking that nobody else has written the policy. A later refresh "+
			"records the version again.",
	)
}

// appendUnconditionalUpdate reports an update sent with no precondition, which is the whole control
// silently absent rather than a step that failed.
//
// It is the majority case on any estate with history: a policy the platform created before it kept
// the counter reports none, so nothing can be compared and the write lands over whatever the policy
// now holds. Nothing else records that — the plan cannot know, the platform answers 204, and the
// apply reads as an ordinary success — so the warning is the only place an operator can see which of
// their policies are unprotected.
func appendUnconditionalUpdate(diags *diag.Diagnostics, id string) {
	diags.AddWarning(
		"AI policy updated without the concurrent-edit check",
		"Terraform holds no usable version for policy "+id+", so this update could not check that the policy "+
			"still held the settings Terraform last read. It has overwritten anything written to the policy "+
			"since the last refresh. Jamf reports no counter for policies created before it started keeping "+
			"one, which is the usual cause.",
	)
}

// appendVersionConflict reports an update the platform refused because the policy is no longer at
// the version Terraform recorded.
//
// It says what was observed rather than naming an editor, because two quite different faults arrive
// as the same 409. Somebody or something else wrote the policy between the refresh and the apply, or
// two Terraform resources manage the same policy — a double import, a module instantiated twice, one
// policy ID behind count — in which case one refresh hands both the same version, the first update
// advances it, and the second is refused. The first clears on a new plan and the second never will.
//
// The remedy is a new plan rather than a re-run, and the difference matters for the shape this
// precondition exists to protect: a saved plan carries the version it was made with, so applying the
// same plan file again resends the value the platform has already refused. Only a plan made after a
// fresh read can carry a version the platform will accept, and it also shows the operator what the
// other write changed before they approve anything over the top of it.
//
// That is worth refusing rather than merging because settings_json is sent whole — the platform
// holds no merge for it, so an update built on a stale read overwrites every setting another editor
// had made.
func appendVersionConflict(diags *diag.Diagnostics, id string) {
	diags.AddError(
		"AI policy is not at the version Terraform last read",
		"Policy "+id+" is no longer at the version Terraform recorded, so Jamf refused this update and changed "+
			"nothing. Make a new plan: its refresh reads the policy as it now stands, and the plan shows you what "+
			"this configuration would alter, including the other write. Applying the same saved plan file again "+
			"fails the same way, because that file carries the version Jamf has already refused. If a new plan is "+
			"refused too, check whether two Terraform resources manage this policy, through a duplicate import or "+
			"a module or count block that names the same policy ID twice. Terraform checks the version because "+
			"settings_json is written whole and Jamf holds no merge for it.",
	)
}
