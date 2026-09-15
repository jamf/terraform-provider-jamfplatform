// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/gatewaystub"
)

// apiError builds an error shaped the way the SDK surfaces a Jamf Security Cloud
// failure, so the translation can be tested without a live tenant.
func apiError(status int, code, description string) error {
	return &jamfplatform.APIResponseError{
		StatusCode: status,
		Errors:     []jamfplatform.ErrorDetail{{Code: code, Description: description}},
	}
}

func TestAppendCreateDiagnostics(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantMatched   bool
		wantSummary   string
		wantAttribute string
		wantDetail    []string
	}{
		{
			name:        "one per tenant",
			err:         apiError(http.StatusConflict, codeConfigAlreadyExists, "A connector already exists for the customer with an incompatible UEM vendor."),
			wantMatched: true,
			wantSummary: "A UEM Connect integration already exists for this tenant",
			// The server blames a vendor mismatch, which is not the cause; the
			// diagnostic has to say so or the user goes looking for the wrong thing.
			wantDetail: []string{"Import the existing integration", "whatever vendor"},
		},
		{
			name:        "connection failure names all three causes",
			err:         apiError(http.StatusBadRequest, codeConnectionFailed, "UEM connection failed."),
			wantMatched: true,
			wantSummary: "Jamf Security Cloud could not reach the Jamf Pro instance",
			wantDetail:  []string{"address is wrong", "not reachable", "credentials are not valid"},
		},
		{
			name:          "group identifier format is attributed to the attribute",
			err:           apiError(http.StatusUnprocessableEntity, codeValidationFailed, "Group ID of JamfPro must start with 'mobile_' or 'computer_' followed by numbers. Invalid group ID: 999999"),
			wantMatched:   true,
			wantSummary:   "Invalid Jamf Pro group identifier",
			wantAttribute: "group_membership_mapping.mappings",
		},
		{
			name:        "other validation failures fall through to a generic error",
			err:         apiError(http.StatusUnprocessableEntity, codeValidationFailed, ": invalid auth configuration for Jamf PRO"),
			wantMatched: true,
			wantSummary: "Jamf Security Cloud rejected the UEM Connect integration",
		},
		{
			name:        "not entitled",
			err:         apiError(http.StatusForbidden, codeNotEntitled, "Not entitled."),
			wantMatched: true,
			wantSummary: "Tenant not entitled to Jamf Security Cloud UEM Connect",
		},
		{
			name:        "an unrecognised code is left to the caller",
			err:         apiError(http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred"),
			wantMatched: false,
		},
		{
			name:        "a non-API error is left to the caller",
			err:         errors.New("dial tcp: connection refused"),
			wantMatched: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			matched := appendCreateDiagnostics(&diags, tc.err)

			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, tc.wantMatched)
			}
			if !tc.wantMatched {
				if diags.HasError() {
					t.Errorf("diagnostics were added for an unrecognised error: %v", diags)
				}
				return
			}
			if !diags.HasError() {
				t.Fatal("no diagnostic was added")
			}

			d := diags.Errors()[0]
			if d.Summary() != tc.wantSummary {
				t.Errorf("summary = %q, want %q", d.Summary(), tc.wantSummary)
			}
			for _, fragment := range tc.wantDetail {
				if !strings.Contains(d.Detail(), fragment) {
					t.Errorf("detail does not mention %q:\n%s", fragment, d.Detail())
				}
			}
			if tc.wantAttribute != "" {
				withPath, ok := d.(diag.DiagnosticWithPath)
				if !ok {
					t.Fatalf("diagnostic is not attached to an attribute; want %s", tc.wantAttribute)
				}
				if got := withPath.Path().String(); got != tc.wantAttribute {
					t.Errorf("attribute path = %q, want %q", got, tc.wantAttribute)
				}
			}
			// Every translated diagnostic quotes what the server said, so an
			// unexpected variant is still traceable from the error text.
			if !strings.Contains(d.Detail(), "Reported by Jamf Security Cloud:") {
				t.Errorf("detail does not quote the server's own message:\n%s", d.Detail())
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// isNotFound has two independent arms — the 404 status, then a Details() scan
		// for codeNotFound. A fixture carrying both cannot tell you which one fired,
		// and the case below named "404 status" used to carry both: mutating
		// http.StatusNotFound to any other status left the whole suite green,
		// including that case. Each arm now has a fixture only it can satisfy.
		{"status without the code", apiError(http.StatusNotFound, "SOMETHING_ELSE", "gone"), true},
		{"code without the status", apiError(http.StatusBadRequest, codeNotFound, "gone"), true},
		{"both the status and the code", apiError(http.StatusNotFound, codeNotFound, "Config with ID '0' doesn't exist"), true},
		// A 404 the gateway produced because it routes nothing at that path is not
		// the integration being absent, and Read acts on isNotFound by deleting the
		// resource from state. The apiError helper above sets no Body, so without
		// this case the helpers.IsGatewayUnrouted guard is unreachable and deleting
		// it leaves every test in the package green.
		{"a gateway-unrouted 404 is not a not-found", &jamfplatform.APIResponseError{StatusCode: http.StatusNotFound, Body: "404 page not found"}, false},
		{"a conflict is not a not-found", apiError(http.StatusConflict, codeConfigAlreadyExists, "exists"), false},
		{"a transport error is not a not-found", errors.New("connection refused"), false},
		{"nil", nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNotFound(tc.err); got != tc.want {
				t.Errorf("isNotFound = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReverseMapping(t *testing.T) {
	in := map[string]string{"a": "1", "b": "2"}
	out := reverseMapping(in)

	if len(out) != 2 || out["1"] != "a" || out["2"] != "b" {
		t.Errorf("reverseMapping = %+v", out)
	}
}

func TestSortedMapKeys(t *testing.T) {
	got := sortedMapKeys(map[string]string{"c": "", "a": "", "b": ""})
	want := []string{"a", "b", "c"}

	if len(got) != len(want) {
		t.Fatalf("got %d keys, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keys = %v, want %v", got, want)
			break
		}
	}
}

// TestAppendMissingIntegrationDiagnostics pins the guard standing in front of
// `page.Results[0]` in the data source. A regression here is a panic, not a
// diagnostic, and it is reachable by reading the data source on a tenant that has
// no integration yet.
func TestAppendMissingIntegrationDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		page    *securitycloud.ConnectorPage
		wantErr bool
	}{
		{"nil page", nil, true},
		{"empty results", &securitycloud.ConnectorPage{Results: []securitycloud.ConnectorConfig{}}, true},
		{"nil results slice", &securitycloud.ConnectorPage{}, true},
		{"one integration", &securitycloud.ConnectorPage{Results: []securitycloud.ConnectorConfig{{ID: "abc"}}}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diags := appendMissingIntegrationDiagnostics(tc.page)

			if diags.HasError() != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v (%v)", diags.HasError(), tc.wantErr, diags)
			}
		})
	}
}

// TestIsNotFound_EdgePageIsNotAbsence covers the one 404 the table above cannot
// reach. An edge error page is marked inside the SDK's unexported transport, so a
// hand-built *APIResponseError can never carry the marker and a table case for it
// would assert nothing — hence the stub.
//
// CloudFront serving a 404 satisfies isNotFound's first arm, the status check, so
// before the !IsEdgeBlocked term Read removed a live UEM Connect integration from
// state and the next refresh emptied it while terraform plan reported a clean
// create at exit 0. Delete's "already gone" branch reported success for the same
// reply.
func TestIsNotFound_EdgePageIsNotAbsence(t *testing.T) {
	t.Parallel()

	err := gatewaystub.ErrorFrom(t, gatewaystub.Reply{
		Status:      http.StatusNotFound,
		ContentType: "text/html",
		Body:        gatewaystub.CloudFrontPage(gatewaystub.CloudFrontNotFound),
	})

	if isNotFound(err) {
		t.Errorf("isNotFound = true for a CloudFront 404 page, so Read removes a UEM Connect "+
			"integration that is still there: %v", err)
	}
}
