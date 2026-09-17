// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/appleprofiles"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// payloadProblemCase pins one appleprofiles.ProblemKind to the diagnostic appendPayloadProblems
// raises for it: which attribute it is reported against, and whether it names the embedded snapshot
// and the raw_component escape hatch.
type payloadProblemCase struct {
	name          string
	payloadType   string
	settings      string
	kind          appleprofiles.ProblemKind
	target        path.Path
	summary       string
	namesSnapshot bool
}

// payloadProblemCases covers every ProblemKind appleprofiles can report for a legacy payload.
// Severity is the point of the table: each finding is an error because Jamf refuses or silently
// discards the write, and nothing else in this package fails if one is downgraded to a warning —
// an unrecognised key or payload type would become advisory again and the payload would apply as a
// no-op with a green plan. The snapshot column pins the other half: only a finding an older table
// could explain offers the escape hatch.
func payloadProblemCases() []payloadProblemCase {
	settingsPath := path.Root("legacy_payloads").AtListIndex(0).AtName("settings")
	typePath := path.Root("legacy_payloads").AtListIndex(0).AtName("payload_type")
	payloadPath := path.Root("legacy_payloads").AtListIndex(0)

	return []payloadProblemCase{
		{
			name:          "UnknownPayloadType",
			payloadType:   "com.example.notarealpayload",
			settings:      `{}`,
			kind:          appleprofiles.UnknownPayloadType,
			target:        typePath,
			summary:       "Unrecognised legacy payload type",
			namesSnapshot: true,
		},
		{
			name:          "MiscasedPayloadType",
			payloadType:   "com.apple.managedclient.preferences",
			settings:      `{}`,
			kind:          appleprofiles.MiscasedPayloadType,
			target:        typePath,
			summary:       "Unrecognised legacy payload type",
			namesSnapshot: false,
		},
		{
			name:          "UnknownKey",
			payloadType:   "com.apple.applicationaccess",
			settings:      `{"bogusKeyOneTwoThree":"x"}`,
			kind:          appleprofiles.UnknownKey,
			target:        settingsPath,
			summary:       "Legacy payload setting does not match Apple's schema",
			namesSnapshot: true,
		},
		{
			name:          "MiscasedKey",
			payloadType:   "com.apple.applicationaccess",
			settings:      `{"AllowCamera":true}`,
			kind:          appleprofiles.MiscasedKey,
			target:        settingsPath,
			summary:       "Legacy payload setting does not match Apple's schema",
			namesSnapshot: false,
		},
		{
			name:          "WrongType",
			payloadType:   "com.apple.applicationaccess",
			settings:      `{"allowScreenShot":"yes"}`,
			kind:          appleprofiles.WrongType,
			target:        settingsPath,
			summary:       "Legacy payload setting does not match Apple's schema",
			namesSnapshot: false,
		},
		{
			name:          "MissingRequiredKey",
			payloadType:   "com.apple.notificationsettings",
			settings:      `{}`,
			kind:          appleprofiles.MissingRequiredKey,
			target:        payloadPath,
			summary:       "Legacy payload is missing a required setting",
			namesSnapshot: true,
		},
		{
			name:          "IntegerOutOfRange",
			payloadType:   "com.apple.AssetCache.managed",
			settings:      `{"CacheLimit":2147483648}`,
			kind:          appleprofiles.IntegerOutOfRange,
			target:        settingsPath,
			summary:       "Legacy payload setting does not match Apple's schema",
			namesSnapshot: false,
		},
	}
}

func TestAppendPayloadProblems_EveryFindingIsAnError(t *testing.T) {
	for _, tc := range payloadProblemCases() {
		t.Run(tc.name, func(t *testing.T) {
			var decoded map[string]any
			if err := json.Unmarshal([]byte(tc.settings), &decoded); err != nil {
				t.Fatalf("failed to decode settings: %v", err)
			}

			if problems := appleprofiles.Validate(tc.payloadType, decoded); len(problems) != 1 || problems[0].Kind != tc.kind {
				t.Fatalf("fixture no longer produces exactly one %v: got %v", tc.kind, problems)
			}

			entry := path.Root("legacy_payloads").AtListIndex(0)
			var diags diag.Diagnostics
			appendPayloadProblems(&diags, tc.payloadType, decoded, entry, entry.AtName("payload_type"), entry.AtName("settings"))

			if len(diags.Warnings()) != 0 {
				t.Errorf("a finding Jamf refuses or silently discards must be an error, not a warning: %v", diags.Warnings())
			}
			if !diags.HasError() {
				t.Fatalf("expected an error diagnostic, got %v", diags)
			}
			if len(diags.Errors()) != 1 {
				t.Fatalf("expected exactly 1 error, got %v", diags.Errors())
			}

			d := diags.Errors()[0]
			if d.Summary() != tc.summary {
				t.Errorf("expected summary %q, got %q", tc.summary, d.Summary())
			}
			withPath, ok := d.(diag.DiagnosticWithPath)
			if !ok {
				t.Fatalf("expected an attribute error carrying a path, got %T", d)
			}
			if !withPath.Path().Equal(tc.target) {
				t.Errorf("expected the error against %s, got %s", tc.target, withPath.Path())
			}
		})
	}
}

func TestAppendPayloadProblems_SnapshotEscapeHatchIsPerBlock(t *testing.T) {
	for _, tc := range payloadProblemCases() {
		t.Run(tc.name, func(t *testing.T) {
			var decoded map[string]any
			if err := json.Unmarshal([]byte(tc.settings), &decoded); err != nil {
				t.Fatalf("failed to decode settings: %v", err)
			}

			entry := path.Root("legacy_payloads").AtListIndex(0)
			var diags diag.Diagnostics
			appendPayloadProblems(&diags, tc.payloadType, decoded, entry, entry.AtName("payload_type"), entry.AtName("settings"))

			if !diags.HasError() {
				t.Fatalf("expected an error diagnostic, got %v", diags)
			}
			detail := diags.Errors()[0].Detail()

			if tc.namesSnapshot {
				if !strings.Contains(detail, "apple/device-management") || !strings.Contains(detail, appleprofiles.ProvenanceSummary()) {
					t.Errorf("expected the snapshot named, got %q", detail)
				}
				if !strings.Contains(detail, "raw_component") {
					t.Errorf("expected the escape hatch offered, got %q", detail)
				}
				if !strings.Contains(detail, "every legacy payload in the same block") || !strings.Contains(detail, "com.jamf.ddm-configuration-profile") {
					t.Errorf("the escape hatch is per block, not per payload — the platform stores a block's payloads as one component; got %q", detail)
				}
				return
			}

			if strings.Contains(detail, "raw_component") || strings.Contains(detail, "apple/device-management") {
				t.Errorf("a finding the snapshot cannot explain must not offer the escape hatch, got %q", detail)
			}
		})
	}
}

// The summaries appendMCXDomainProblems reports for the two counts it refuses. They are named here
// rather than restated at each assertion so that the empty case can be asserted absent wherever the
// multi-domain case is asserted present, and the other way round.
const (
	severalPreferenceDomainsSummary  = "Custom settings payload sets more than one preference domain"
	emptyPreferenceDictionarySummary = "Custom settings payload sets no preference domain"
)

// mcxSettings renders a custom settings payload's settings for the named preference domains, each
// forcing one key of its own. That is the shape the Jamf Pro profile editor writes. Passing no
// domain at all renders the empty dictionary, which the provider refuses during plan.
func mcxSettings(t *testing.T, domains ...string) string {
	t.Helper()

	content := make(map[string]any, len(domains))
	for i, domain := range domains {
		content[domain] = map[string]any{
			"Forced": []any{map[string]any{"mcx_preference_settings": map[string]any{"orderedKey": i}}},
		}
	}

	encoded, err := json.Marshal(map[string]any{mcxPreferenceDomainsKey: content})
	if err != nil {
		t.Fatalf("marshalling a custom settings payload: %v", err)
	}
	return string(encoded)
}

// legacyPayloadElementType is the object type of one entry in a block's legacy_payloads list.
func legacyPayloadElementType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"payload_type": types.StringType,
		"settings":     types.StringType,
	}}
}

// legacyPayloadListValue builds a block's legacy_payloads list from (payload type, settings) pairs,
// so a test can drive ValidateList the way the framework does.
func legacyPayloadListValue(t *testing.T, payloads ...[2]string) types.List {
	t.Helper()

	elementType := legacyPayloadElementType()
	elements := make([]attr.Value, 0, len(payloads))
	for _, payload := range payloads {
		element, diags := types.ObjectValue(elementType.AttrTypes, map[string]attr.Value{
			"payload_type": types.StringValue(payload[0]),
			"settings":     types.StringValue(payload[1]),
		})
		if diags.HasError() {
			t.Fatalf("building a legacy payload element: %v", diags)
		}
		elements = append(elements, element)
	}

	list, diags := types.ListValue(elementType, elements)
	if diags.HasError() {
		t.Fatalf("building the legacy payload list: %v", diags)
	}
	return list
}

// legacyPayloadsListPath is the path the framework passes ValidateList.
func legacyPayloadsListPath() path.Path {
	return path.Root("component_blocks").AtListIndex(0).AtName("legacy_payloads")
}

// validateBlockLegacyPayloads runs the block list validator over a value and returns its
// diagnostics.
func validateBlockLegacyPayloads(t *testing.T, list types.List) diag.Diagnostics {
	t.Helper()

	var resp validator.ListResponse
	blockLegacyPayloadSchemaValidator().ValidateList(
		context.Background(),
		validator.ListRequest{Path: legacyPayloadsListPath(), ConfigValue: list},
		&resp,
	)
	return resp.Diagnostics
}

// validateFlatLegacyPayloads runs the deprecated top-level validator over the same payloads, where
// settings arrive as objects rather than JSON strings.
func validateFlatLegacyPayloads(t *testing.T, payloads ...[2]string) diag.Diagnostics {
	t.Helper()

	items := make([]any, 0, len(payloads))
	for _, payload := range payloads {
		var settings map[string]any
		if err := json.Unmarshal([]byte(payload[1]), &settings); err != nil {
			t.Fatalf("decoding settings for the flat carrier: %v", err)
		}
		items = append(items, map[string]any{"payload_type": payload[0], "settings": settings})
	}

	value, err := helpers.JSONToTerraformDynamic(items)
	if err != nil {
		t.Fatalf("building the flat legacy_payloads value: %v", err)
	}

	var resp validator.DynamicResponse
	flatLegacyPayloadSchemaValidator().ValidateDynamic(
		context.Background(),
		validator.DynamicRequest{Path: path.Root("legacy_payloads"), ConfigValue: value},
		&resp,
	)
	return resp.Diagnostics
}

// TestLegacyPayloadValidator_SeveralPreferenceDomainsIsAnError covers the finding no other layer can
// produce. Apple declares the dictionary the domains sit under as free-form, so appleprofiles stops
// descending at it, and Jamf stores every domain faithfully — the loss happens on the device, which
// applies one domain and discards the rest while the deploy reports SUCCEEDED. Without this check
// the apply is green, every later plan settles, and two of three domains never reach a Mac.
func TestLegacyPayloadValidator_SeveralPreferenceDomainsIsAnError(t *testing.T) {
	t.Parallel()

	settings := mcxSettings(t, "com.example.first", "com.example.second")

	for _, tc := range []struct {
		name  string
		diags func(*testing.T) diag.Diagnostics
		want  path.Path
	}{
		{
			name: "block carrier",
			diags: func(t *testing.T) diag.Diagnostics {
				t.Helper()
				return validateBlockLegacyPayloads(t, legacyPayloadListValue(t, [2]string{mcxPayloadType, settings}))
			},
			want: legacyPayloadsListPath().AtListIndex(0).AtName("settings"),
		},
		{
			name: "flat carrier",
			diags: func(t *testing.T) diag.Diagnostics {
				t.Helper()
				return validateFlatLegacyPayloads(t, [2]string{mcxPayloadType, settings})
			},
			want: path.Root("legacy_payloads"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diags := tc.diags(t)
			if !diags.HasError() {
				t.Fatalf("two preference domains in one payload produced no error: %v", diags)
			}
			if len(diags.Warnings()) != 0 {
				t.Errorf("the finding must be an error, not a warning: %v", diags.Warnings())
			}
			if len(diags.Errors()) != 1 {
				t.Fatalf("expected exactly 1 error, got %v", diags.Errors())
			}

			reported := diags.Errors()[0]
			if reported.Summary() != severalPreferenceDomainsSummary {
				t.Errorf("summary = %q", reported.Summary())
			}
			for _, domain := range []string{"com.example.first", "com.example.second"} {
				if !strings.Contains(reported.Detail(), domain) {
					t.Errorf("detail does not name %s: %s", domain, reported.Detail())
				}
			}

			withPath, ok := reported.(diag.DiagnosticWithPath)
			if !ok {
				t.Fatalf("expected an attribute error carrying a path, got %T", reported)
			}
			if !withPath.Path().Equal(tc.want) {
				t.Errorf("path = %s, want %s", withPath.Path(), tc.want)
			}
		})
	}
}

// TestLegacyPayloadValidator_OnePreferenceDomainPasses is the other half: the check must not reject
// the shape the whole fix tells an author to write. A rule that fired on a single domain would make
// every custom settings payload unauthorable.
func TestLegacyPayloadValidator_OnePreferenceDomainPasses(t *testing.T) {
	t.Parallel()

	settings := mcxSettings(t, "com.example.only")

	if diags := validateBlockLegacyPayloads(t, legacyPayloadListValue(t, [2]string{mcxPayloadType, settings})); diags.HasError() {
		t.Errorf("the block carrier rejected a single preference domain: %v", diags.Errors())
	}
	if diags := validateFlatLegacyPayloads(t, [2]string{mcxPayloadType, settings}); diags.HasError() {
		t.Errorf("the flat carrier rejected a single preference domain: %v", diags.Errors())
	}
}

// TestLegacyPayloadValidator_NoPreferenceDomainsIsRefused pins the shape that cannot round-trip.
// Jamf discards an empty dictionary: a payload sent with `PayloadContent: {}` reads back carrying
// only the stamped metadata keys, so the read finds no settings, flattenBlockLegacyPayloads writes a
// null, and Terraform fails the apply with "Provider produced inconsistent result after apply".
// State cannot hold what the author wrote, whatever the provider stores for it, so the refusal has
// to arrive at plan time. The summary is the empty case's own, not the multi-domain one, so an
// operator reads the count that applies.
func TestLegacyPayloadValidator_NoPreferenceDomainsIsRefused(t *testing.T) {
	t.Parallel()

	settings := mcxSettings(t)

	for _, tc := range []struct {
		name  string
		diags func(*testing.T) diag.Diagnostics
	}{
		{
			name: "block carrier",
			diags: func(t *testing.T) diag.Diagnostics {
				t.Helper()
				return validateBlockLegacyPayloads(t, legacyPayloadListValue(t, [2]string{mcxPayloadType, settings}))
			},
		},
		{
			name: "flat carrier",
			diags: func(t *testing.T) diag.Diagnostics {
				t.Helper()
				return validateFlatLegacyPayloads(t, [2]string{mcxPayloadType, settings})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diags := tc.diags(t)
			if !diags.HasError() {
				t.Fatalf("an empty preference dictionary produced no error: %v", diags)
			}
			if len(diags.Warnings()) != 0 {
				t.Errorf("the finding must be an error, not a warning: %v", diags.Warnings())
			}
			if len(diags.Errors()) != 1 {
				t.Fatalf("expected exactly 1 error, got %v", diags.Errors())
			}
			if summary := diags.Errors()[0].Summary(); summary != emptyPreferenceDictionarySummary {
				t.Errorf("summary = %q, want the empty case's own summary", summary)
			}
		})
	}
}

// TestLegacyPayloadValidator_AbsentPreferenceDictionaryStaysApplesFinding keeps the domain-count
// check out of the way of the one that was already there. A payload carrying no dictionary at all is
// a missing required key, which names the key and offers the escape hatch; reporting the domain
// count instead would replace a precise diagnostic with a vaguer one.
//
// Both count summaries are asserted absent, the empty one above all: an absent dictionary and an
// empty dictionary are one line apart in mcxPreferenceDomains, and conflating them would report
// Jamf discarding a dictionary the author never wrote.
func TestLegacyPayloadValidator_AbsentPreferenceDictionaryStaysApplesFinding(t *testing.T) {
	t.Parallel()

	diags := validateBlockLegacyPayloads(t, legacyPayloadListValue(t, [2]string{mcxPayloadType, `{}`}))
	if !diags.HasError() {
		t.Fatal("a custom settings payload with no preference dictionary produced no error")
	}
	for _, reported := range diags.Errors() {
		switch reported.Summary() {
		case severalPreferenceDomainsSummary, emptyPreferenceDictionarySummary:
			t.Errorf("a domain count was reported for a payload that carries no dictionary: %s", reported.Detail())
		}
	}
	if summary := diags.Errors()[0].Summary(); summary != "Legacy payload is missing a required setting" {
		t.Errorf("summary = %q, want Apple's missing-required-key finding", summary)
	}
}
