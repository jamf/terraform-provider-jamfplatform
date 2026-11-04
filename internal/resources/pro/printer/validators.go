// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// useGenericPPDConfigValidator enforces the bidirectional cross-field rule
// between `use_generic` and the PPD trio (`ppd`, `ppd_path`, `ppd_contents`)
// at plan time, before apply. The Jamf Pro server applies the same rule
// silently on write — without the validator, the user's config would diverge
// from the persisted state on every apply.
//
// The rule (confirmed by the §13.2 audit, see local-testing/printers/
// artifacts):
//
//   - `use_generic = true` or unset → the server uses the bundled macOS
//     Generic.ppd and ignores any user-supplied PPD trio values (an explicit
//     true over the API discards them; a filename stored by a UI upload is
//     kept server-side but inert, and the provider reads it back as null). So `ppd`,
//     `ppd_path`, and `ppd_contents` must all be unset in the user's config.
//
//   - `use_generic = false` → the server expects the user to supply a
//     concrete PPD via `ppd_path` (the gate field). If `ppd_path` is omitted
//     the server flips `use_generic` back to true and falls back to the
//     bundled PPD — `ppd` and `ppd_contents` alone are not sufficient.
//
// `use_generic` is Optional+Computed with Default(true); the validator runs
// against the user's config so an omitted attribute reads as null (the
// schema Default has not been applied yet) and is treated as the implicit
// `true`.
//
// Mirrors `ibeacon`'s includeAnyMajorMinorConfigValidator — see
// STYLE_GUIDE §Cross-field validation for why a custom validator is needed
// instead of off-the-shelf framework validators.
type useGenericPPDConfigValidator struct{}

// Description returns a plain-text description of the validator.
func (useGenericPPDConfigValidator) Description(context.Context) string {
	return "use_generic = true (or unset) forbids ppd, ppd_path, and ppd_contents; use_generic = false requires ppd_path"
}

// MarkdownDescription returns the markdown description.
func (v useGenericPPDConfigValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time cross-field check.
func (useGenericPPDConfigValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data PrinterResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.UseGeneric.IsUnknown() {
		return
	}

	// Treat null (omitted attribute) as the implicit Default(true).
	useGeneric := data.UseGeneric.IsNull() || data.UseGeneric.ValueBool()

	if useGeneric {
		forbidden := map[string]bool{
			"ppd":          isSet(data.PPD),
			"ppd_path":     isSet(data.PPDPath),
			"ppd_contents": isSet(data.PPDContents.StringValue),
		}
		for attr, set := range forbidden {
			if !set {
				continue
			}
			resp.Diagnostics.AddAttributeError(
				path.Root(attr),
				fmt.Sprintf("%s forbidden when use_generic is true", attr),
				fmt.Sprintf("`%s` cannot be set when `use_generic` is true (or omitted) — the Jamf Pro server ignores any user-supplied PPD and uses the bundled Generic.ppd. To supply a custom PPD set `use_generic = false` and populate `ppd_path` (and optionally `ppd` and `ppd_contents`).", attr),
			)
		}
		return
	}

	// use_generic = false
	// Defer when ppd_path is unknown (variable/for_each/resource-driven):
	// config-time validation cannot see its eventual value, so treating unknown
	// as "missing" would false-error on every non-literal config. isSet() treats
	// unknown as not-set, so guard explicitly here.
	if data.PPDPath.IsUnknown() {
		return
	}
	if !isSet(data.PPDPath) {
		resp.Diagnostics.AddAttributeError(
			path.Root("ppd_path"),
			"ppd_path required when use_generic is false",
			"`ppd_path` is required when `use_generic = false`. Jamf Pro uses `ppd_path` as the gate that distinguishes a user-supplied PPD from the bundled Generic.ppd; without it Jamf Pro silently flips `use_generic` back to true. To match any specific printer set `ppd_path` to the path of an installed PPD file (e.g. `/Library/Printers/PPDs/Contents/Resources/HP DeskJet 2600 series.ppd`).",
		)
	}
}

// noLiteralSentinelValidator rejects the literal server sentinel
// `categoryUnassignedSentinel` as a user-supplied `category` value. The
// server stores this exact string for printers with no category assigned
// and the state builder decodes it back to null, so a user writing it
// literally either means "I have a category named exactly that" (which
// would 409 since the server reserves the string) or has copy-pasted from
// a refreshed state file. Either way, surfacing the error at plan time is
// kinder than a 409 at apply.
type noLiteralSentinelValidator struct{}

// Description returns a plain-text description of the validator.
func (noLiteralSentinelValidator) Description(context.Context) string {
	return fmt.Sprintf("category cannot be the literal string %q — leave the attribute unset to mean \"no category\"", categoryUnassignedSentinel)
}

// MarkdownDescription returns the markdown description.
func (v noLiteralSentinelValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString rejects exactly the sentinel string.
func (noLiteralSentinelValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() == categoryUnassignedSentinel {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"category cannot be the Jamf Pro sentinel",
			fmt.Sprintf("Jamf Pro uses %q as an internal sentinel for printers with no category assigned. To mean \"no category\" leave the `category` attribute unset; the provider hides the sentinel from state. To assign a real category, supply its display name (e.g. `category = \"Printers\"`).", categoryUnassignedSentinel),
		)
	}
}

// bundledGenericPPDPathValidator rejects the bundled Generic.ppd path as a
// `ppd_path`. Jamf Pro trims surrounding whitespace and then treats that path
// as the generic-PPD gate, setting use_generic back to true and leaving apply
// with different use_generic, ppd and ppd_path values than the plan. The path
// is invalid in every mode, since use_generic = true already forbids any
// ppd_path, so the rule reads no sibling. It compares the trimmed value
// because a file() or heredoc source adds a trailing newline that
// stringvalidator.NoneOf would not match.
type bundledGenericPPDPathValidator struct{}

// Description returns a plain-text description of the validator.
func (bundledGenericPPDPathValidator) Description(context.Context) string {
	return "ppd_path cannot be the bundled Generic.ppd path"
}

// MarkdownDescription returns the markdown description.
func (v bundledGenericPPDPathValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString rejects the bundled path, ignoring surrounding whitespace.
func (bundledGenericPPDPathValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if strings.TrimSpace(req.ConfigValue.ValueString()) == bundledGenericPPDPath {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"ppd_path is the bundled Generic.ppd path",
			"Jamf Pro treats the bundled macOS Generic.ppd path as a request to use the generic PPD and sets `use_generic` back to true, so the printer no longer matches this configuration. Set `use_generic = true` and omit `ppd`, `ppd_path` and `ppd_contents`, or point `ppd_path` at a specific installed PPD file.",
		)
	}
}

// Compile-time interface assertions.
var (
	_ validator.String         = bundledGenericPPDPathValidator{}
	_ resource.ConfigValidator = useGenericPPDConfigValidator{}
	_ validator.String         = noLiteralSentinelValidator{}
)

// isSet reports whether a TF String attribute is non-null, non-unknown, and
// has a non-empty value. Validators use this to distinguish "user supplied a
// value" from "user omitted the attribute."
func isSet(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}
