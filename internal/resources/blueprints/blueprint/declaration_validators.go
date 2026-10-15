// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	appledeclarations "github.com/jamf/jamfplatform-go-sdk/appleschemas/declarations"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint/components"
)

// declarationSchemaValidator checks each declaration's payload against Apple's published schemas
// during plan, which is the only place these mistakes are caught.
//
// Jamf Pro validates none of it: the blueprints service stores whatever it is given, answers 201,
// and a deploy reports SUCCEEDED — while the editor renders the offending key blank and the device
// never receives it. Every finding is therefore an error, and there is no switch to soften one. A
// declaration that should not be checked belongs in raw_component, which exists for exactly that
// and round-trips a JSON-encoded configuration unchanged.
//
// Applied to both declaration-bearing components. apple_declarations is the typed surface;
// custom_declarations delivers identical declarations with identical exposure, so leaving it
// unchecked would only relocate the silent failure.
//
// They differ in shape, not in rules, and the shape decides which method runs. apple_declarations
// is a list of declarations, so ValidateList addresses a finding to the exact element and can
// range-check a $PAYLOAD_n reference. custom_declarations is an object wrapping a set, which has no
// stable index, so ValidateObject lands a finding on the collection and names the declaration type.
type declarationSchemaValidator struct{}

// appleDeclarationsSchemaValidator validates the apple_declarations list.
func appleDeclarationsSchemaValidator() validator.List {
	return declarationSchemaValidator{}
}

// customDeclarationsSchemaValidator validates the custom_declarations component.
func customDeclarationsSchemaValidator() validator.Object {
	return declarationSchemaValidator{}
}

func (v declarationSchemaValidator) Description(ctx context.Context) string {
	return v.MarkdownDescription(ctx)
}

func (v declarationSchemaValidator) MarkdownDescription(context.Context) string {
	_, release := appledeclarations.Provenance()
	return fmt.Sprintf(
		"each declaration's payload must match Apple's declared keys for its declaration type (schemas from %s)",
		release,
	)
}

// ValidateList checks every declaration in the apple_declarations list, addressing each finding to
// the element it came from. Only this surface checks $PAYLOAD_n references, because only a list has
// the positions they name.
//
// An element Terraform has yet to compute cannot be read into the model, and that is not an error —
// the wire decides it, which is why the elements are read with unhandled unknowns allowed. An
// element that reads but carries an unknown type or payload still warns, from
// validateDeclarationPayload. What is left once the unknown element is tolerated is the model and
// the element type having diverged, and that is reported rather than swallowed, for the reason
// ValidateObject gives: returning clean on it would leave this validator passing every
// configuration for ever with no diagnostic to say why.
func (v declarationSchemaValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if !helpers.IsConfiguredValue(req.ConfigValue) {
		return
	}

	var declarations []AppleDeclarationModel
	if diags := req.ConfigValue.ElementsAs(ctx, &declarations, true); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	for i, declaration := range declarations {
		at := req.Path.AtListIndex(i).AtName("payload")
		resp.Diagnostics.Append(validateDeclarationPayload(
			types.StringNull(), declaration.Type, declaration.Payload, at, "",
		)...)
		resp.Diagnostics.Append(validatePayloadReferences(
			declaration.Payload, len(declarations), at,
		)...)
	}
}

// ValidateObject checks every declaration the custom_declarations component carries.
//
// A failure to read the component into its model is reported rather than swallowed:
// UnhandledUnknownAsEmpty already absorbs a value Terraform has yet to compute, and every model
// field is a types.String, which carries unknown natively, so the only cause left is the model and
// the object type having diverged. Returning clean on that would leave this validator passing every
// configuration forever with no diagnostic to say why.
func (v declarationSchemaValidator) ValidateObject(ctx context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if !helpers.IsConfiguredValue(req.ConfigValue) {
		return
	}

	var component components.CustomDeclarationsComponent
	diags := req.ConfigValue.As(ctx, &component, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	base := req.Path.AtName("declaration")
	for _, declaration := range component.Declarations {
		resp.Diagnostics.Append(validateDeclarationPayload(
			declaration.Kind, declaration.Type, declaration.Payload,
			base, declaration.Type.ValueString(),
		)...)
	}
}

// validateDeclarationPayload checks one declaration's payload.
//
// authoredKind is the configured kind on the surface that carries one, and null on the surface that
// derives the kind from the declaration type; a null suppresses the pairing check rather than faking
// agreement with the type. attribution names the declaration in the diagnostic when the path cannot
// point at it precisely, and is empty when it can.
//
// A type or payload Terraform has yet to compute cannot be checked, and the check never runs again:
// attribute validators run at plan, and the apply path marshals a payload without validating it. So
// an unresolved value warns rather than passing silently. A warning is right rather than an error
// because the configuration may be perfectly valid — the operator only needs to know the guarantee
// the payload's description promises did not apply here.
func validateDeclarationPayload(authoredKind, declarationType, payload types.String, at path.Path, attribution string) diag.Diagnostics {
	var diags diag.Diagnostics

	if !helpers.IsConfiguredValue(declarationType) || !helpers.IsConfiguredValue(payload) {
		if declarationType.IsUnknown() || payload.IsUnknown() {
			diags.AddAttributeWarning(at,
				"Declaration payload was not checked against Apple's schemas",
				attributionPrefix(attribution)+"The payload is not known until apply, so the plan-time schema "+
					"check did not run for it. Jamf Pro accepts and discards an unrecognised key without "+
					"reporting anything, so this declaration is delivered unchecked.",
			)
		}
		return diags
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload.ValueString()), &decoded); err != nil {
		diags.AddAttributeError(at,
			"Declaration payload is not a JSON object",
			fmt.Sprintf("%sCould not decode the payload: %s. Author it with jsonencode({ ... }).",
				attributionPrefix(attribution), helpers.APIErrorDetail(err)),
		)
		return diags
	}

	declared := declarationType.ValueString()
	kind := appledeclarations.KindForType(declared)
	if helpers.IsConfiguredValue(authoredKind) {
		kind = authoredKind.ValueString()
	}
	for _, problem := range appledeclarations.Validate(kind, declared, decoded) {
		summary, detail := renderDeclarationProblem(problem, declared, attribution)
		diags.AddAttributeError(at, summary, detail)
	}

	return diags
}

// payloadReference matches a reference from one declaration's payload to another declaration in the
// same component.
var payloadReference = regexp.MustCompile(`\$PAYLOAD_(\d+)`)

// validatePayloadReferences reports a $PAYLOAD_n reference naming a position the component does not
// hold.
//
// Apple declares an asset reference as a plain string, so no schema check can catch this, and the
// provider is the only thing that knows the positions: payload_key is derived from the 1-based
// index. Ordered components only, because a set has no stable index for a reference to name — which
// is also why the typed surface is a list in the first place.
func validatePayloadReferences(payload types.String, declarations int, at path.Path) diag.Diagnostics {
	var diags diag.Diagnostics

	if !helpers.IsConfiguredValue(payload) {
		return diags
	}

	reported := make(map[string]bool)
	for _, match := range payloadReference.FindAllStringSubmatch(payload.ValueString(), -1) {
		if reported[match[0]] || payloadReferenceInRange(match[1], declarations) {
			continue
		}
		reported[match[0]] = true
		diags.AddAttributeError(at,
			"Declaration payload references a declaration that does not exist",
			fmt.Sprintf(
				"The payload references %s, but this component holds %s. A reference names a declaration by "+
					"its position, counting from 1 in the order they are listed here, so this one resolves to "+
					"nothing and the declaration is delivered bound to no asset.",
				match[0], countedDeclarations(declarations),
			),
		)
	}

	return diags
}

// payloadReferenceInRange reports whether a $PAYLOAD_n reference names a position the component
// holds. A number too large to parse is out of range by definition.
func payloadReferenceInRange(digits string, declarations int) bool {
	position, err := strconv.Atoi(digits)
	if err != nil {
		return false
	}
	return position >= 1 && position <= declarations
}

// countedDeclarations renders a declaration count for a diagnostic.
func countedDeclarations(count int) string {
	if count == 1 {
		return "1 declaration"
	}
	return fmt.Sprintf("%d declarations", count)
}

// renderDeclarationProblem turns a finding into a diagnostic, naming the schema snapshot whenever
// the finding could be caused by the provider's tables being older than what Jamf Pro offers.
func renderDeclarationProblem(problem appledeclarations.Problem, declarationType, attribution string) (summary, detail string) {
	summary = "Declaration does not match Apple's schema"
	switch problem.Kind {
	case appledeclarations.UnknownDeclarationType, appledeclarations.MiscasedDeclarationType:
		summary = "Unknown Apple declaration type"
	case appledeclarations.KindMismatch:
		summary = "Declaration kind does not match its type"
	case appledeclarations.UnknownKey, appledeclarations.MiscasedKey:
		summary = "Unknown key in declaration payload"
	case appledeclarations.MissingRequiredKey:
		summary = "Declaration payload is missing a required key"
	case appledeclarations.UnknownStatusItem:
		summary = "Unknown status item"
	}

	parts := make([]string, 0, 6)
	if attribution != "" {
		parts = append(parts, "In declaration "+attribution+":")
	}
	if problem.Path != "" {
		parts = append(parts, "At "+problem.Path+":")
	}
	parts = append(parts, problem.Detail)

	if problem.Kind == appledeclarations.UnknownKey {
		if hint := declaredKeyHint(declarationType, problem.Path); hint != "" {
			parts = append(parts, hint)
		}
	}
	if problem.SeedOnly {
		parts = append(parts, "This part of Apple's schema comes from a pre-release branch, so it is newer than the released one.")
	}
	if staleTableSuspect(problem.Kind) {
		parts = append(parts, fmt.Sprintf(
			"The provider's schemas come from apple/device-management %s. If Apple has published this since, "+
				"upgrade the provider; to deliver a declaration without these checks, move it to raw_component.",
			appledeclarations.ProvenanceSummary(),
		))
	}

	return summary, strings.Join(parts, " ")
}

// declaredKeyHint names the keys Apple declares for a declaration type, so an unknown-key
// diagnostic offers somewhere to go rather than only saying no.
//
// Top-level keys only: a nested path would have to be walked back through array indices to reach the
// right dictionary, and listing the wrong dictionary's keys is worse than listing none. Long lists
// are truncated with a count, so a declaration carrying dozens of keys does not bury the finding.
func declaredKeyHint(declarationType, problemPath string) string {
	if declarationType == "" || strings.ContainsAny(problemPath, ".[") {
		return ""
	}
	declaration, ok := appledeclarations.Lookup(declarationType)
	if !ok || len(declaration.Keys) == 0 {
		return ""
	}

	names := slices.Sorted(maps.Keys(declaration.Keys))
	if len(names) > 12 {
		return fmt.Sprintf("Apple declares %d keys here, among them %s.", len(names), strings.Join(names[:12], ", "))
	}
	return "Apple declares " + strings.Join(names, ", ") + "."
}

// staleTableSuspect reports whether a finding could be caused by the embedded schemas being older
// than what Jamf Pro offers, rather than by a genuine mistake.
//
// Every finding whose truth depends on the snapshot's revision qualifies, which is more than the
// name-based ones: Apple widens an enum, widens a range, and relaxes a required key between
// revisions, so a snapshot older than the tenant can report any of those against a value that
// works. That is the same set the SDK's apple-schemas.yml calls out for review when a refresh narrows one.
// What is left out is a wrong value type and a miscased name, each wrong against every revision
// that declares the key at all.
func staleTableSuspect(kind appledeclarations.ProblemKind) bool {
	switch kind {
	case appledeclarations.UnknownDeclarationType, appledeclarations.UnknownKey,
		appledeclarations.UnknownStatusItem, appledeclarations.NotInEnum,
		appledeclarations.OutOfRange, appledeclarations.MissingRequiredKey:
		return true
	default:
		return false
	}
}

// attributionPrefix renders the optional declaration attribution as a sentence opener.
func attributionPrefix(attribution string) string {
	if attribution == "" {
		return ""
	}
	return "In declaration " + attribution + ": "
}
