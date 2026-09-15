// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appinstalleractions

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/gatewaystub"
)

// fakeAppInstallerClient answers every call with one canned error, recording the
// arguments. A nil err is a success.
type fakeAppInstallerClient struct {
	err          error
	tenantCalls  int
	deployments  []string
	computers    [][2]string
	versionMoves [][2]string
}

func (f *fakeAppInstallerClient) RetryAppInstallerInstallationsV1(ctx context.Context) error {
	f.tenantCalls++
	return f.err
}

func (f *fakeAppInstallerClient) RetryAppInstallerDeploymentInstallationsV1(ctx context.Context, id string) error {
	f.deployments = append(f.deployments, id)
	return f.err
}

func (f *fakeAppInstallerClient) RetryAppInstallerDeploymentComputerInstallationV1(ctx context.Context, id string, computerID string) error {
	f.computers = append(f.computers, [2]string{id, computerID})
	return f.err
}

func (f *fakeAppInstallerClient) UpdateAppInstallerDeploymentVersionV1(ctx context.Context, id string, request *pro.AppTitleVersion) error {
	version := ""
	if request != nil && request.Version != nil {
		version = *request.Version
	}
	f.versionMoves = append(f.versionMoves, [2]string{id, version})
	return f.err
}

// emptyNotFound is the "nothing to retry" response: 404 with an EMPTY errors
// array. Wire-verified against a GA tenant.
func emptyNotFound() error {
	return &jamfplatform.APIResponseError{StatusCode: http.StatusNotFound, Method: http.MethodPost, URL: "https://example.invalid"}
}

// invalidIDBadRequest is Jamf Pro refusing a MALFORMED deployment ID, before it
// looks anything up. Wire-verified: a non-numeric or zero ID answers this on
// every App Installer retry endpoint.
func invalidIDBadRequest() error {
	return &jamfplatform.APIResponseError{
		StatusCode: http.StatusBadRequest,
		Method:     http.MethodPost,
		URL:        "https://example.invalid",
		Errors: []jamfplatform.ErrorDetail{{
			Code:        "INVALID_ID",
			Field:       "deploymentId",
			Description: "id field must be string of positive numeric value or -1",
		}},
	}
}

func serverError() error {
	return &jamfplatform.APIResponseError{StatusCode: http.StatusInternalServerError, Method: http.MethodPost, URL: "https://example.invalid"}
}

// invokeResponse returns an InvokeResponse whose SendProgress is a no-op, which
// the framework always supplies in production.
func invokeResponse() *action.InvokeResponse {
	return &action.InvokeResponse{SendProgress: func(action.InvokeProgressEvent) {}}
}

// actionConfig builds a tfsdk.Config from the action's real schema, filling
// every attribute null except those supplied in attrs.
func actionConfig(t *testing.T, a action.Action, attrs map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	schema := schemaOf(t, a).Schema
	objType := schema.Type().TerraformType(context.Background()).(tftypes.Object)
	values := map[string]tftypes.Value{}
	for name, typ := range objType.AttributeTypes {
		if v, ok := attrs[name]; ok {
			values[name] = v
			continue
		}
		values[name] = tftypes.NewValue(typ, nil)
	}
	return tfsdk.Config{Schema: schema, Raw: tftypes.NewValue(objType, values)}
}

// retryInstallations invokes the single-deployment retry against fake, with the
// given deployment ID and optional computer IDs.
func retryInstallations(t *testing.T, fake *fakeAppInstallerClient, id string, computerIDs []string) *action.InvokeResponse {
	t.Helper()
	a := NewRetryInstallationsAction().(*RetryInstallationsAction)
	a.client = fake

	attrs := map[string]tftypes.Value{"deployment_id": tftypes.NewValue(tftypes.String, id)}
	if computerIDs != nil {
		elems := make([]tftypes.Value, 0, len(computerIDs))
		for _, c := range computerIDs {
			elems = append(elems, tftypes.NewValue(tftypes.String, c))
		}
		attrs["computer_ids"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, elems)
	}

	resp := invokeResponse()
	a.Invoke(context.Background(), action.InvokeRequest{Config: actionConfig(t, a, attrs)}, resp)
	return resp
}

func retryAll(t *testing.T, fake *fakeAppInstallerClient) *action.InvokeResponse {
	t.Helper()
	a := NewRetryAllInstallationsAction().(*RetryAllInstallationsAction)
	a.client = fake

	resp := invokeResponse()
	a.Invoke(context.Background(), action.InvokeRequest{}, resp)
	return resp
}

func updateVersion(t *testing.T, fake *fakeAppInstallerClient, id, version string) *action.InvokeResponse {
	t.Helper()
	a := NewUpdateVersionAction().(*UpdateVersionAction)
	a.client = fake

	resp := invokeResponse()
	a.Invoke(context.Background(), action.InvokeRequest{Config: actionConfig(t, a, map[string]tftypes.Value{
		"deployment_id": tftypes.NewValue(tftypes.String, id),
		"version":       tftypes.NewValue(tftypes.String, version),
	})}, resp)
	return resp
}

// warningsOnly asserts the invocation produced exactly one warning and no error.
func warningsOnly(t *testing.T, resp *action.InvokeResponse) {
	t.Helper()
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected a warning, got errors: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 {
		t.Fatalf("expected exactly one warning, got %v", resp.Diagnostics)
	}
}

// requireError asserts the invocation FAILED and that the detail carries text.
func requireError(t *testing.T, resp *action.InvokeResponse, wantSubstring string) {
	t.Helper()
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error, got %v", resp.Diagnostics)
	}
	if wantSubstring == "" {
		return
	}
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), wantSubstring) {
			return
		}
	}
	t.Fatalf("no error detail contained %q: %v", wantSubstring, resp.Diagnostics.Errors())
}

func TestRetryInstallations_EmptyNotFoundIsWarning(t *testing.T) {
	fake := &fakeAppInstallerClient{err: emptyNotFound()}
	warningsOnly(t, retryInstallations(t, fake, "9", nil))
	if len(fake.deployments) != 1 || fake.deployments[0] != "9" {
		t.Errorf("expected one retry of deployment 9, got %v", fake.deployments)
	}
}

// A malformed deployment_id is refused `400 INVALID_ID` BEFORE Jamf Pro looks
// the deployment up. Classifying it as "nothing to retry" would report success
// on an apply that did nothing and name two causes that are both wrong, so it
// MUST be an error carrying Jamf Pro's own field description. This is the
// regression guard for the finding that the shared helpers.IsNotFoundError —
// which matches 400 INVALID_ID as well as 404 — must not be used here.
func TestRetryInstallations_InvalidIDIsError(t *testing.T) {
	fake := &fakeAppInstallerClient{err: invalidIDBadRequest()}
	requireError(t, retryInstallations(t, fake, "Composer", nil), "INVALID_ID")
}

func TestRetryInstallations_ServerErrorIsError(t *testing.T) {
	fake := &fakeAppInstallerClient{err: serverError()}
	requireError(t, retryInstallations(t, fake, "9", nil), "500")
}

func TestRetryInstallations_SuccessIsSilent(t *testing.T) {
	fake := &fakeAppInstallerClient{}
	resp := retryInstallations(t, fake, "9", nil)
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %v", resp.Diagnostics)
	}
}

// A per-computer empty 404 is that computer having nothing to retry and must not
// abandon the rest of the list, so it is a single trailing warning.
func TestRetryInstallations_PerComputerEmptyNotFoundIsWarning(t *testing.T) {
	fake := &fakeAppInstallerClient{err: emptyNotFound()}
	warningsOnly(t, retryInstallations(t, fake, "9", []string{"1", "2"}))
	if len(fake.computers) != 2 {
		t.Errorf("expected both computers to be attempted, got %v", fake.computers)
	}
}

func TestRetryInstallations_PerComputerInvalidIDIsError(t *testing.T) {
	fake := &fakeAppInstallerClient{err: invalidIDBadRequest()}
	resp := retryInstallations(t, fake, "Composer", []string{"1", "2"})
	requireError(t, resp, "INVALID_ID")
	if len(fake.computers) != 1 {
		t.Errorf("a refused ID must abandon the list, got %v", fake.computers)
	}
}

func TestRetryAllInstallations_EmptyNotFoundIsWarning(t *testing.T) {
	fake := &fakeAppInstallerClient{err: emptyNotFound()}
	warningsOnly(t, retryAll(t, fake))
	if fake.tenantCalls != 1 {
		t.Errorf("expected one tenant-wide retry, got %d", fake.tenantCalls)
	}
}

// The tenant-wide endpoint takes no ID, but the classifier is shared, so the
// guard is kept here too: only an empty 404 may be downgraded to a warning.
func TestRetryAllInstallations_InvalidIDIsError(t *testing.T) {
	fake := &fakeAppInstallerClient{err: invalidIDBadRequest()}
	requireError(t, retryAll(t, fake), "INVALID_ID")
}

func TestRetryAllInstallations_ServerErrorIsError(t *testing.T) {
	fake := &fakeAppInstallerClient{err: serverError()}
	requireError(t, retryAll(t, fake), "500")
}

func TestRetryAllInstallations_SuccessIsSilent(t *testing.T) {
	fake := &fakeAppInstallerClient{}
	resp := retryAll(t, fake)
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %v", resp.Diagnostics)
	}
}

// A 404 on the version move is a deployment that does not exist, which is an
// error — but naming the forward-only and MANUAL preconditions there would be
// the whole message and none of it the cause.
func TestUpdateVersion_NotFoundNamesTheDeployment(t *testing.T) {
	fake := &fakeAppInstallerClient{err: emptyNotFound()}
	resp := updateVersion(t, fake, "9", "11.31.1")
	requireError(t, resp, "found no App Installer deployment 9")
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "update_behavior") {
			t.Errorf("a missing deployment must not be blamed on update_behavior: %s", d.Detail())
		}
	}
}

// A forward-only refusal is a 400 whose own description names both versions, so
// the preconditions stay appended there.
func TestUpdateVersion_ForwardOnlyRefusalKeepsPreconditions(t *testing.T) {
	fake := &fakeAppInstallerClient{err: &jamfplatform.APIResponseError{
		StatusCode: http.StatusBadRequest,
		Errors: []jamfplatform.ErrorDetail{{
			Code:        "INVALID_FIELD",
			Field:       "version",
			Description: "current version '11.31.1' cannot be updated to the new version '10.41.0'",
		}},
	}}
	requireError(t, updateVersion(t, fake, "9", "10.41.0"), "update_behavior")
}

func TestUpdateVersion_SuccessSendsTheVersion(t *testing.T) {
	fake := &fakeAppInstallerClient{}
	resp := updateVersion(t, fake, "9", "11.31.1")
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %v", resp.Diagnostics)
	}
	if len(fake.versionMoves) != 1 || fake.versionMoves[0] != [2]string{"9", "11.31.1"} {
		t.Errorf("unexpected version moves: %v", fake.versionMoves)
	}
}

// Every action must refuse to run rather than dereference a nil client.
func TestInvokeWithoutConfigureIsAnError(t *testing.T) {
	for _, a := range []action.Action{
		NewRetryInstallationsAction(),
		NewRetryAllInstallationsAction(),
		NewUpdateVersionAction(),
	} {
		resp := invokeResponse()
		a.Invoke(context.Background(), action.InvokeRequest{}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("%T invoked without a client must error", a)
		}
	}
}

// isNothingToRetry must key off BOTH a 404 status and an empty error body: a
// non-API error, a 404 that carries details, and any other status are all
// genuine failures.
func TestIsNothingToRetry(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"nil":                    {nil, false},
		"empty 404":              {emptyNotFound(), true},
		"400 INVALID_ID":         {invalidIDBadRequest(), false},
		"500":                    {serverError(), false},
		"plain error":            {errors.New("connection refused"), false},
		"404 with error details": {&jamfplatform.APIResponseError{StatusCode: http.StatusNotFound, Errors: []jamfplatform.ErrorDetail{{Code: "OBJECT_NOT_FOUND"}}}, false},
		"403 unrouted":           {&jamfplatform.APIResponseError{StatusCode: http.StatusForbidden, Errors: []jamfplatform.ErrorDetail{{Code: "BAD_PERMISSIONS"}}}, false},
		"wrapped empty 404":      {errors.Join(emptyNotFound()), true},
		// A 404 the gateway produced because it routes nothing at that path carries
		// no error details either, so it satisfies this classifier's own tell — an
		// empty 404 — exactly, and it is not a deployment with nothing to retry.
		// emptyNotFound sets no Body, and an empty body is precisely not the gateway
		// text, so without this case the helpers.IsGatewayUnrouted guard is
		// unreachable and deleting it leaves every test in the package green.
		"gateway-unrouted 404": {&jamfplatform.APIResponseError{StatusCode: http.StatusNotFound, Body: "404 page not found"}, false},
	} {
		if got := isNothingToRetry(tc.err); got != tc.want {
			t.Errorf("%s: isNothingToRetry = %v, want %v", name, got, tc.want)
		}
	}
}

// TestIsNothingToRetry_EdgePageIsNotNothingToRetry covers the one 404 the table
// above cannot reach. An edge error page is marked inside the SDK's unexported
// transport, so a hand-built *APIResponseError can never carry the marker and a
// table case for it would assert nothing — hence the stub.
//
// It pins the outcome, not one term: an edge page must never succeed the action
// with "nothing needed retrying", because that tells the operator the failed
// installations are clear when the request never reached Jamf Pro. Two mechanisms
// now hold that — the !IsEdgeBlocked guard, and the fact that SDK v1.0.0 condenses
// an edge page into one synthetic error detail, so the classifier's empty-errors
// tell already misses it. Removing either alone leaves this test green, which is
// why the guard's doc comment says so rather than leaving a reader hunting for the
// test that proves it.
func TestIsNothingToRetry_EdgePageIsNotNothingToRetry(t *testing.T) {
	t.Parallel()

	err := gatewaystub.ErrorFrom(t, gatewaystub.Reply{
		Status:      http.StatusNotFound,
		ContentType: "text/html",
		Body:        gatewaystub.CloudFrontPage(gatewaystub.CloudFrontNotFound),
	})

	if isNothingToRetry(err) {
		t.Errorf("isNothingToRetry = true for a CloudFront 404 page, so the retry action reports "+
			"success and issues no retry: %v", err)
	}
}
