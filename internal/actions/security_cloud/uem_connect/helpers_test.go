// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uemconnectactions

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/gatewaystub"
)

func apiError(status int, code, description string) error {
	return &jamfplatform.APIResponseError{
		StatusCode: status,
		Errors:     []jamfplatform.ErrorDetail{{Code: code, Description: description}},
	}
}

func TestAppendInvokeDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantMatched bool
		wantSummary string
		wantDetail  []string
	}{
		{
			name:        "disabled integration names the setting to change",
			err:         apiError(http.StatusConflict, codeConnectorDisabled, "UEM integration is disabled for connector '6a91'"),
			wantMatched: true,
			wantSummary: "UEM Connect is disabled, so it cannot synchronize",
			// The server names the integration's identifier; what the caller needs
			// is the attribute that has to change.
			wantDetail: []string{"enabled = true"},
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
			name:        "a transport error is left to the caller",
			err:         errors.New("connection refused"),
			wantMatched: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			matched := appendInvokeDiagnostics(&diags, tc.err)

			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, tc.wantMatched)
			}
			if !tc.wantMatched {
				if diags.HasError() {
					t.Errorf("diagnostics added for an unrecognised error: %v", diags)
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
			if !strings.Contains(d.Detail(), "Reported by Jamf Security Cloud:") {
				t.Errorf("detail does not quote the server's own message:\n%s", d.Detail())
			}
		})
	}
}

// TestNotEntitledUsesTheSDKConstant pins that the one code the SDK generates is
// taken from there rather than restated, and that the one it does not is still
// absent — so if a future spec adds it, this says so.
func TestNotEntitledUsesTheSDKConstant(t *testing.T) {
	if codeNotEntitled != securitycloud.ApiErrorItemCodeNotEntitled {
		t.Errorf("codeNotEntitled = %q; it must come from the SDK constant", codeNotEntitled)
	}

	for _, v := range securitycloud.ApiErrorItemCodeValues() {
		if v == codeConnectorDisabled {
			t.Errorf("the SDK now generates a constant for %q — use it instead of the literal", codeConnectorDisabled)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"404", apiError(http.StatusNotFound, "NOT_FOUND", "gone"), true},
		// A 404 the gateway produced because it routes nothing at that path is not
		// the integration being absent. The apiError helper above sets no Body, so
		// without this case the helpers.IsGatewayUnrouted guard is unreachable and
		// deleting it leaves every test in the package green.
		{"gateway-unrouted 404 is not a not-found", &jamfplatform.APIResponseError{StatusCode: http.StatusNotFound, Body: "404 page not found"}, false},
		{"conflict is not a not-found", apiError(http.StatusConflict, codeConnectorDisabled, "disabled"), false},
		{"transport error", errors.New("connection refused"), false},
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

// TestEnsureClientFailsClosed pins that Invoke refuses to run with no client rather
// than panicking on a nil dereference.
func TestEnsureClientFailsClosed(t *testing.T) {
	var a uemConnectAction
	var resp action.InvokeResponse

	if a.ensureClient(&resp) {
		t.Error("ensureClient returned true with no client")
	}
	if !resp.Diagnostics.HasError() {
		t.Error("ensureClient added no diagnostic")
	}
}

// TestAppendDeployDiagnostics covers each translated code, and the two cases that
// must not be translated.
func TestAppendDeployDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		osValue     string
		groups      []string
		wantMatched bool
		wantSummary string
		wantDetail  []string
	}{
		{
			name:        "unknown code names both readings",
			err:         apiError(http.StatusNotFound, codeActivationProfileNotFound, "Activation profile with the given code, or its UEM template for the requested uem and platform, was not found."),
			osValue:     "ios_supervised",
			wantMatched: true,
			wantSummary: "Activation profile not found",
			// Jamf Security Cloud cannot distinguish an unknown code from a code
			// with no configuration profile for the requested operating system, so
			// the diagnostic must not pick one.
			wantDetail: []string{"the code is wrong", "no configuration profile for this operating system"},
		},
		{
			name:        "misconfigured names the group kind for an iOS value",
			err:         apiError(http.StatusUnprocessableEntity, codeConnectorMisconfigured, "UEM is misconfigured"),
			osValue:     "ios_supervised",
			groups:      []string{"29"},
			wantMatched: true,
			wantSummary: "A named Jamf Pro group cannot be used for this deployment",
			// The server blames the connector and names neither the field nor the
			// group; the replacement has to say which kind was expected and list
			// the candidates.
			wantDetail: []string{"mobile device groups", "computer group", "29", "does not exist"},
		},
		{
			name:        "misconfigured flips the group kind for macos",
			err:         apiError(http.StatusUnprocessableEntity, codeConnectorMisconfigured, "UEM is misconfigured"),
			osValue:     macOSValue,
			groups:      []string{"1"},
			wantMatched: true,
			wantSummary: "A named Jamf Pro group cannot be used for this deployment",
			wantDetail:  []string{"computer groups", "mobile device group"},
		},
		{
			name:        "misconfigured with no groups still explains itself",
			err:         apiError(http.StatusUnprocessableEntity, codeConnectorMisconfigured, "UEM is misconfigured"),
			osValue:     "ios_byod",
			wantMatched: true,
			wantSummary: "A named Jamf Pro group cannot be used for this deployment",
			wantDetail:  []string{"no groups were named"},
		},
		{
			name:        "disabled integration names the setting to change",
			err:         apiError(http.StatusConflict, codeConnectorDisabled, "UEM integration is disabled"),
			osValue:     "ios_supervised",
			wantMatched: true,
			wantSummary: "UEM Connect is disabled, so it cannot deploy",
			wantDetail:  []string{"enabled = true"},
		},
		{
			name:        "not connected points at the Jamf Pro credentials",
			err:         apiError(http.StatusUnprocessableEntity, codeConnectorNotConnected, "Connector is not connected"),
			osValue:     "ios_supervised",
			wantMatched: true,
			wantSummary: "UEM Connect is not connected to Jamf Pro",
			wantDetail:  []string{"Jamf Pro credentials"},
		},
		{
			name:        "ambiguous target sends the operator to the console",
			err:         apiError(http.StatusConflict, codeMultipleActivationProfiles, "More than one active activation profile"),
			osValue:     "ios_supervised",
			wantMatched: true,
			wantSummary: "More than one activation profile is active, so the deployment target is ambiguous",
			// Nothing in the provider can list them, so the diagnostic must not
			// suggest looking one up.
			wantDetail: []string{"Nothing in this provider can list them"},
		},
		{
			name:        "malformed group id names the prefix mistake",
			err:         apiError(http.StatusUnprocessableEntity, codeValidationFailed, "Invalid scoping group ID (expected integer): 'computer_29'"),
			osValue:     "ios_supervised",
			wantMatched: true,
			wantSummary: "A named Jamf Pro group ID is not a number",
			wantDetail:  []string{"computer_", "decided by `os`"},
		},
		{
			name:        "entitlement failure is shared with synchronize",
			err:         apiError(http.StatusForbidden, codeNotEntitled, "Not entitled"),
			osValue:     "ios_supervised",
			wantMatched: true,
			wantSummary: "Tenant not entitled to Jamf Security Cloud UEM Connect",
			wantDetail:  []string{"authenticated successfully"},
		},
		{
			// VALIDATION_FAILED is this service's catch-all, so an enum violation
			// arrives under the same code. Translating it as a group problem would
			// be actively misleading, and plan-time validation means it should
			// never arrive at all.
			name:        "enum validation failure is not claimed as a group problem",
			err:         apiError(http.StatusUnprocessableEntity, codeValidationFailed, `JSON parse error: Cannot deserialize value of type ActivationProfilePlatform from String "NOPE": not one of the values accepted for Enum class`),
			osValue:     "ios_supervised",
			wantMatched: false,
		},
		{
			name:        "a non-API error is left to the caller",
			err:         errors.New("dial tcp: connection refused"),
			osValue:     "ios_supervised",
			wantMatched: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			matched := appendDeployDiagnostics(&diags, tc.err, "ky2lgv2t", tc.osValue, tc.groups)

			if matched != tc.wantMatched {
				t.Fatalf("matched = %v, want %v (diags: %v)", matched, tc.wantMatched, diags)
			}
			if !tc.wantMatched {
				if diags.HasError() {
					t.Errorf("unrecognised error produced diagnostics: %v", diags)
				}
				return
			}
			if !diags.HasError() {
				t.Fatal("matched but produced no error diagnostic")
			}

			summary := diags.Errors()[0].Summary()
			if summary != tc.wantSummary {
				t.Errorf("summary = %q, want %q", summary, tc.wantSummary)
			}
			detail := diags.Errors()[0].Detail()
			for _, want := range tc.wantDetail {
				if !strings.Contains(detail, want) {
					t.Errorf("detail does not contain %q:\n%s", want, detail)
				}
			}
		})
	}
}

// TestAppendDeployDiagnostics_NamesTheCode pins that the activation profile code
// reaches the diagnostic. A caller who mistyped it needs to see what was sent,
// because nothing in the provider can list the valid ones.
func TestAppendDeployDiagnostics_NamesTheCode(t *testing.T) {
	var diags diag.Diagnostics
	appendDeployDiagnostics(&diags,
		apiError(http.StatusNotFound, codeActivationProfileNotFound, "not found"),
		"ky2lgv2t", "ios_supervised", nil)

	if !diags.HasError() {
		t.Fatal("expected a diagnostic")
	}
	if !strings.Contains(diags.Errors()[0].Detail(), "ky2lgv2t") {
		t.Errorf("detail does not name the code that was sent:\n%s", diags.Errors()[0].Detail())
	}
}

// TestDeployedMessage covers the progress line, which is the only report a
// successful deploy produces — actions hold no state, so if the message does not
// say what scope was added, nothing does.
func TestDeployedMessage(t *testing.T) {
	withGroups := deployedMessage("ky2lgv2t", "ios_supervised", []string{"1", "2"})
	for _, want := range []string{"ky2lgv2t", "ios_supervised", "1, 2", "added"} {
		if !strings.Contains(withGroups, want) {
			t.Errorf("message does not contain %q: %s", want, withGroups)
		}
	}

	// The no-groups line must not read as success at scoping, and must not claim
	// the scope was merely left alone either: wire probing established that a first
	// deployment with no groups creates a configuration profile scoped to nothing,
	// which "unchanged" on its own would describe as a non-event. Both outcomes have
	// to be named, because deployedMessage cannot tell which one happened.
	without := deployedMessage("ky2lgv2t", "macos", nil)
	for _, want := range []string{"no groups were named", "keeps the scope it had", "reaches no devices"} {
		if !strings.Contains(without, want) {
			t.Errorf("no-groups message does not contain %q: %s", want, without)
		}
	}
}

// TestGroupIDsFromSet covers the conversion, including the null case that has to
// become an omitted field rather than an empty array.
//
// The distinction is not cosmetic on this endpoint, though it is invisible: the wire
// probe found an empty array and an omitted field both leave the scope untouched,
// so sending one for the other changes nothing today. It is still worth getting
// right, because the spec claims both mean "every group" and a server that ever
// starts honouring that would make the two diverge.
func TestGroupIDsFromSet(t *testing.T) {
	tests := []struct {
		name string
		set  types.Set
		want []string
	}{
		{
			name: "null yields nothing",
			set:  types.SetNull(types.StringType),
			want: nil,
		},
		{
			name: "unknown yields nothing",
			set:  types.SetUnknown(types.StringType),
			want: nil,
		},
		{
			name: "values are sorted for a stable request body",
			set: types.SetValueMust(types.StringType, []attr.Value{
				types.StringValue("30"), types.StringValue("2"), types.StringValue("1"),
			}),
			want: []string{"1", "2", "30"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := groupIDsFromSet(context.Background(), tc.set, &diags)

			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got %v, want %v", got, tc.want)
					break
				}
			}
		})
	}
}

// TestIsNotFound_EdgePageIsNotAbsence covers the one 404 the table above cannot
// reach. An edge error page is marked inside the SDK's unexported transport, so a
// hand-built *APIResponseError can never carry the marker and a table case for it
// would assert nothing — hence the stub.
//
// CloudFront serving a 404 carries the status isNotFound keys off, so before the
// !IsEdgeBlocked term an action told the operator the integration did not exist
// for a request that never arrived at Jamf Security Cloud.
func TestIsNotFound_EdgePageIsNotAbsence(t *testing.T) {
	t.Parallel()

	err := gatewaystub.ErrorFrom(t, gatewaystub.Reply{
		Status:      http.StatusNotFound,
		ContentType: "text/html",
		Body:        gatewaystub.CloudFrontPage(gatewaystub.CloudFrontNotFound),
	})

	if isNotFound(err) {
		t.Errorf("isNotFound = true for a CloudFront 404 page, so the action reports an absent "+
			"integration for a request that never reached Jamf: %v", err)
	}
}
