// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// apiError builds the error shape the SDK produces for a failed call, so the tests below exercise
// the same unwrapping path the CRUD methods do.
func apiError(status int, code, field, description string) error {
	return fmt.Errorf("CreatePolicy: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "POST",
		URL:        "https://eu.api.jamfcloud.com/ai/governance/policies/v1/policies",
		Errors: []jamfplatform.ErrorDetail{{
			Code:        code,
			Field:       field,
			Description: description,
		}},
	})
}

// TestIsNotFound pins that the one code covering a missing policy, a malformed identifier and an
// already-archived policy is recognised in all three cases — they are indistinguishable on the wire.
func TestIsNotFound(t *testing.T) {
	if !isNotFound(apiError(404, codePolicyNotFound, "", "Policy not found")) {
		t.Error("POLICY_NOT_FOUND must be recognised as absent")
	}
	if isNotFound(apiError(422, codeToolIDUnknown, "toolId", "Unknown tool identifier")) {
		t.Error("an unrelated code must not read as absent")
	}
	if isNotFound(errors.New("dial tcp: connection refused")) {
		t.Error("a transport error must not read as absent")
	}
}

func TestHasCode(t *testing.T) {
	err := apiError(409, codeNoDraftToPublish, "", "No draft version available to publish")
	if !hasCode(err, codeNoDraftToPublish) {
		t.Error("NO_DRAFT_TO_PUBLISH must be recognised")
	}
	if hasCode(err, codePolicyNotFound) {
		t.Error("an absent code must not match")
	}
	if hasCode(nil, codePolicyNotFound) {
		t.Error("a nil error must not match")
	}
}

// TestAppendWriteDiagnostics pins each translated code against the body captured during the wire
// probe, and pins which Terraform attribute it blames — the platform names wire fields, which a
// practitioner has never typed.
func TestAppendWriteDiagnostics(t *testing.T) {
	cases := []struct {
		name          string
		err           error
		wantMatched   bool
		wantAttribute string
		wantSummary   string
		wantMentions  string
	}{
		{
			name:          "unknown tool",
			err:           apiError(422, codeToolIDUnknown, "toolId", "Unknown tool identifier"),
			wantMatched:   true,
			wantAttribute: "tool_id",
			wantSummary:   "Unknown AI tool",
			wantMentions:  "jamfplatform_ai_governance_tools",
		},
		{
			name:          "unknown schema version",
			err:           apiError(422, codeSchemaVersionUnknown, "schemaVersion", "Schema version is not available"),
			wantMatched:   true,
			wantAttribute: "schema_version",
			wantSummary:   "Unknown settings schema version",
			wantMentions:  "schema_versions",
		},
		{
			name:          "settings type failure carries a JSON pointer",
			err:           apiError(422, codeSchemaValidationFailed, "/verbose", "string found, boolean expected"),
			wantMatched:   true,
			wantAttribute: "settings_json",
			wantSummary:   "Settings do not match the tool's schema",
			wantMentions:  "/verbose",
		},
		{
			name:          "settings failure with no field named",
			err:           apiError(422, codeSchemaValidationFailed, "", "array found, object expected"),
			wantMatched:   true,
			wantAttribute: "settings_json",
			wantSummary:   "Settings do not match the tool's schema",
			wantMentions:  "array found, object expected",
		},
		{
			name:         "field validation failure",
			err:          apiError(400, codeValidationFailed, "settings", "must not be null"),
			wantMatched:  true,
			wantSummary:  "Jamf rejected the policy",
			wantMentions: "\"settings\"",
		},
		{
			name:        "unrecognised code is left to the caller",
			err:         apiError(403, "BAD_PERMISSIONS", "", "The given token was not authorized"),
			wantMatched: false,
		},
		{
			name:        "transport error is left to the caller",
			err:         errors.New("dial tcp: connection refused"),
			wantMatched: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var diags diag.Diagnostics
			matched := appendWriteDiagnostics(&diags, c.err)

			if matched != c.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, c.wantMatched)
			}
			if !c.wantMatched {
				if diags.HasError() {
					t.Errorf("unmatched error produced diagnostics: %v", diags)
				}
				return
			}
			if !diags.HasError() {
				t.Fatal("expected an error diagnostic")
			}

			first := diags.Errors()[0]
			if first.Summary() != c.wantSummary {
				t.Errorf("summary = %q, want %q", first.Summary(), c.wantSummary)
			}
			if !strings.Contains(first.Detail(), c.wantMentions) {
				t.Errorf("detail does not mention %q: %s", c.wantMentions, first.Detail())
			}
			if c.wantAttribute != "" {
				withPath, ok := first.(diag.DiagnosticWithPath)
				if !ok {
					t.Fatalf("diagnostic has no attribute path, want %s", c.wantAttribute)
				}
				if got := withPath.Path().String(); got != c.wantAttribute {
					t.Errorf("attribute path = %q, want %q", got, c.wantAttribute)
				}
			}
		})
	}
}

// TestRequestContextNotProvidedIsNotTranslated pins mappings.go's decision: the gateway's own
// pre-routing failure is left raw, because a client that reaches it disagrees with its own scope and
// the untranslated error says more than a rewritten one would.
func TestRequestContextNotProvidedIsNotTranslated(t *testing.T) {
	var diags diag.Diagnostics
	if appendWriteDiagnostics(&diags, apiError(400, codeRequestContextNotProvided, "", "The request context could not be detected.")) {
		t.Error("REQUEST_CONTEXT_NOT_PROVIDED must not be translated")
	}
}

func TestSchemaFailureDetail(t *testing.T) {
	withField := schemaFailureDetail("/permissions/allow/0", "string found, object expected")
	if !strings.Contains(withField, "The setting at /permissions/allow/0") {
		t.Errorf("detail should locate the setting: %s", withField)
	}
	withoutField := schemaFailureDetail("", "array found, object expected")
	if !strings.Contains(withoutField, "The settings were rejected") {
		t.Errorf("detail should fall back to the whole object: %s", withoutField)
	}
}

func TestQuoteOrUnnamed(t *testing.T) {
	if got := quoteOrUnnamed("settings"); got != `"settings"` {
		t.Errorf("got %q", got)
	}
	if got := quoteOrUnnamed("   "); got != "it did not name" {
		t.Errorf("whitespace-only field should read as unnamed, got %q", got)
	}
	if got := quoteOrUnnamed(""); got != "it did not name" {
		t.Errorf("empty field should read as unnamed, got %q", got)
	}
}
