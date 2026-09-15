// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// entraListItem is a registry summary row for a Microsoft Entra ID provider.
// The registry reports the legacy wire value, which is the point of the
// provider_name assertions below.
func entraListItem() pro.CloudIDPCommonResponse {
	return pro.CloudIDPCommonResponse{
		ID:           "idp-entra-1",
		DisplayName:  "Corporate Entra ID",
		ProviderName: wireProviderAzure,
		Enabled:      true,
	}
}

// googleListItem is a registry summary row for a Google Secure LDAP provider.
func googleListItem() pro.CloudIDPCommonResponse {
	return pro.CloudIDPCommonResponse{
		ID:           "idp-google-1",
		DisplayName:  "Google Secure LDAP",
		ProviderName: providerGoogle,
		Enabled:      true,
	}
}

// fullAzureConfiguration is a complete Entra ID read response, including the
// mappings Jamf Pro generates whether or not anybody asked for them.
func fullAzureConfiguration() *pro.AzureConfiguration {
	return &pro.AzureConfiguration{
		CloudIDPCommon: &pro.CloudIDPCommon{
			ID:           "idp-entra-1",
			DisplayName:  "Corporate Entra ID",
			ProviderName: wireProviderAzure,
		},
		Server: &pro.AzureServerConfiguration{
			TenantID:                                 "11111111-2222-3333-4444-555555555555",
			SearchTimeout:                            45,
			Enabled:                                  true,
			MembershipCalculationOptimizationEnabled: true,
			TransitiveMembershipEnabled:              true,
			TransitiveMembershipUserField:            "userPrincipalName",
			TransitiveDirectoryMembershipEnabled:     true,
			Type:                                     "PUBLIC",
			Migrated:                                 true,
			DeprecatedConsent:                        false,
			Mappings: &pro.AzureMappings{
				UserID:    "id",
				UserName:  "userPrincipalName",
				RealName:  "displayName",
				Email:     "mail",
				GroupID:   "id",
				GroupName: "displayName",
			},
		},
	}
}

// TestHydrateListedCloudIdentityProvider_EntraIDHydrates covers the finding this
// path exists for: a state built from the registry summary alone leaves
// `entra_id` empty, and the resource's cross-field validator then refuses the
// generated configuration. Every scalar the Entra ID block carries must come
// back from the per-item read.
func TestHydrateListedCloudIdentityProvider_EntraIDHydrates(t *testing.T) {
	reads := 0
	read := func(ctx context.Context, id string) (*pro.AzureConfiguration, error) {
		reads++
		if id != "idp-entra-1" {
			t.Errorf("read called with id %q; want the registry id", id)
		}
		return fullAzureConfiguration(), nil
	}

	state, skip := hydrateListedCloudIdentityProvider(context.Background(), entraListItem(), read)
	if skip != nil {
		t.Fatalf("Entra ID provider must hydrate; got skip %+v", skip)
	}
	if reads != 1 {
		t.Errorf("read call count: got %d, want 1", reads)
	}
	if state == nil {
		t.Fatal("hydrated state must not be nil")
	}
	if state.ID.ValueString() != "idp-entra-1" {
		t.Errorf("ID: got %q", state.ID.ValueString())
	}
	if state.DisplayName.ValueString() != "Corporate Entra ID" {
		t.Errorf("DisplayName: got %q", state.DisplayName.ValueString())
	}
	if state.ProviderName.ValueString() != providerEntraID {
		t.Errorf("ProviderName must be the Terraform-facing value; got %q", state.ProviderName.ValueString())
	}
	if state.Google != nil {
		t.Errorf("Google block must stay nil on an Entra ID provider")
	}
	if state.Azure == nil {
		t.Fatal("entra_id block must be populated, otherwise the generated configuration cannot plan")
	}

	az := state.Azure
	if az.TenantID.ValueString() != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("TenantID: got %q", az.TenantID.ValueString())
	}
	if az.SearchTimeout.ValueInt64() != 45 {
		t.Errorf("SearchTimeout: got %d", az.SearchTimeout.ValueInt64())
	}
	if !az.Enabled.ValueBool() {
		t.Errorf("Enabled: got false")
	}
	if !az.MembershipCalculationOptimizationEnabled.ValueBool() {
		t.Errorf("MembershipCalculationOptimizationEnabled: got false")
	}
	if !az.TransitiveMembershipEnabled.ValueBool() {
		t.Errorf("TransitiveMembershipEnabled: got false")
	}
	if az.TransitiveMembershipUserField.ValueString() != "userPrincipalName" {
		t.Errorf("TransitiveMembershipUserField: got %q", az.TransitiveMembershipUserField.ValueString())
	}
	if !az.TransitiveDirectoryMembershipEnabled.ValueBool() {
		t.Errorf("TransitiveDirectoryMembershipEnabled: got false")
	}
	if az.Type.ValueString() != "PUBLIC" {
		t.Errorf("Type: got %q", az.Type.ValueString())
	}
	if !az.Migrated.ValueBool() {
		t.Errorf("Migrated: got false")
	}
	if az.DeprecatedConsent.ValueBool() {
		t.Errorf("DeprecatedConsent: got true")
	}
	if !state.Timeouts.IsNull() {
		t.Errorf("Timeouts must be a null object, not carried over from anywhere")
	}
}

// TestHydrateListedCloudIdentityProvider_MappingsHydrate pins that the
// generated-config path carries Jamf Pro's own generated mappings. It used to
// pin the opposite: `mappings` is Optional and not Computed, and nobody has
// authored it on the generation path, so the block was left absent — which
// agreed with what import produced but made the mappings unrepresentable in a
// generated configuration. Import now hydrates them too, so both paths agree on
// the populated shape instead (issue #391).
func TestHydrateListedCloudIdentityProvider_MappingsHydrate(t *testing.T) {
	read := func(ctx context.Context, id string) (*pro.AzureConfiguration, error) {
		return fullAzureConfiguration(), nil
	}

	state, skip := hydrateListedCloudIdentityProvider(context.Background(), entraListItem(), read)
	if skip != nil {
		t.Fatalf("Entra ID provider must hydrate; got skip %+v", skip)
	}
	if state.Azure == nil {
		t.Fatal("entra_id block must be populated")
	}
	if state.Azure.Mappings == nil {
		t.Fatal("mappings must hydrate on the generated-config path")
	}
	if got := state.Azure.Mappings.UserName.ValueString(); got != "userPrincipalName" {
		t.Errorf("mappings.user_name = %q, want userPrincipalName", got)
	}
}

// TestHydrateListedCloudIdentityProvider_GoogleIsSkipped covers a Google
// provider being dropped without a read at all: its keystore is Required and
// write-only, so there is nothing a read could supply and no configuration that
// could plan.
func TestHydrateListedCloudIdentityProvider_GoogleIsSkipped(t *testing.T) {
	read := func(ctx context.Context, id string) (*pro.AzureConfiguration, error) {
		t.Fatal("a Google provider must not be read through the Entra ID endpoint")
		return nil, nil
	}

	state, skip := hydrateListedCloudIdentityProvider(context.Background(), googleListItem(), read)
	if state != nil {
		t.Errorf("a Google provider must carry no generated state; got %+v", state)
	}
	if skip == nil {
		t.Fatal("a Google provider must report a skip so the operator is warned")
	}
	if skip.reason != skipUnrepresentableGoogle {
		t.Errorf("skip reason: got %v, want skipUnrepresentableGoogle", skip.reason)
	}
	if skip.id != "idp-google-1" || skip.name != "Google Secure LDAP" {
		t.Errorf("skip must name the provider; got id %q name %q", skip.id, skip.name)
	}
}

// TestHydrateListedCloudIdentityProvider_ReadFailureIsSkipped covers a provider
// the registry enumerated but the per-item read refused. It is dropped rather
// than streamed with an empty block, and the error travels with it so the
// warning can name a cause.
func TestHydrateListedCloudIdentityProvider_ReadFailureIsSkipped(t *testing.T) {
	refused := errors.New("403 forbidden")
	read := func(ctx context.Context, id string) (*pro.AzureConfiguration, error) {
		return nil, refused
	}

	state, skip := hydrateListedCloudIdentityProvider(context.Background(), entraListItem(), read)
	if state != nil {
		t.Errorf("a failed read must carry no generated state; got %+v", state)
	}
	if skip == nil {
		t.Fatal("a failed read must report a skip")
	}
	if skip.reason != skipUnreadable {
		t.Errorf("skip reason: got %v, want skipUnreadable", skip.reason)
	}
	if !errors.Is(skip.err, refused) {
		t.Errorf("skip must carry the read error; got %v", skip.err)
	}
}

// TestHydrateListedCloudIdentityProvider_EmptyResponseIsSkipped covers a read
// that succeeds but returns no connection settings. Folding that in would write
// the empty block this whole path exists to avoid, so it is treated as a failed
// read.
func TestHydrateListedCloudIdentityProvider_EmptyResponseIsSkipped(t *testing.T) {
	for name, response := range map[string]*pro.AzureConfiguration{
		"nil response":  nil,
		"no server":     {CloudIDPCommon: &pro.CloudIDPCommon{ID: "idp-entra-1"}},
		"empty payload": {},
	} {
		t.Run(name, func(t *testing.T) {
			read := func(ctx context.Context, id string) (*pro.AzureConfiguration, error) {
				return response, nil
			}

			state, skip := hydrateListedCloudIdentityProvider(context.Background(), entraListItem(), read)
			if state != nil {
				t.Errorf("must carry no generated state; got %+v", state)
			}
			if skip == nil {
				t.Fatal("must report a skip")
			}
			if skip.reason != skipUnreadable {
				t.Errorf("skip reason: got %v, want skipUnreadable", skip.reason)
			}
			if !errors.Is(skip.err, errNoEntraConfiguration) {
				t.Errorf("skip error: got %v, want errNoEntraConfiguration", skip.err)
			}
		})
	}
}

// TestSkippedCloudIdentityProviderDiagnostics_Google asserts the operator sees a
// warning that names the provider and says what to do instead. A dropped result
// generates no HCL and is otherwise silent, so this warning is the only signal.
func TestSkippedCloudIdentityProviderDiagnostics_Google(t *testing.T) {
	diags := skippedCloudIdentityProviderDiagnostics(
		[]skippedCloudIdentityProvider{{id: "idp-google-1", name: "Google Secure LDAP", reason: skipUnrepresentableGoogle}},
		nil,
	)

	if len(diags) != 1 {
		t.Fatalf("expected one warning; got %d: %v", len(diags), diags)
	}
	if diags.HasError() {
		t.Errorf("a skipped provider must warn, never fail the query: %v", diags)
	}

	detail := diags[0].Detail()
	for _, want := range []string{"Google Secure LDAP", "idp-google-1", "client certificate", "terraform import", "keystore"} {
		if !strings.Contains(detail, want) {
			t.Errorf("warning detail must mention %q; got:\n%s", want, detail)
		}
	}
}

// TestSkippedCloudIdentityProviderDiagnostics_Unreadable asserts the failed-read
// warning names the provider and the error that dropped it.
func TestSkippedCloudIdentityProviderDiagnostics_Unreadable(t *testing.T) {
	diags := skippedCloudIdentityProviderDiagnostics(
		nil,
		[]skippedCloudIdentityProvider{{id: "idp-entra-1", name: "Corporate Entra ID", reason: skipUnreadable, err: errors.New("403 forbidden")}},
	)

	if len(diags) != 1 {
		t.Fatalf("expected one warning; got %d: %v", len(diags), diags)
	}
	if diags.HasError() {
		t.Errorf("a skipped provider must warn, never fail the query: %v", diags)
	}

	detail := diags[0].Detail()
	for _, want := range []string{"Corporate Entra ID", "idp-entra-1", "403 forbidden"} {
		if !strings.Contains(detail, want) {
			t.Errorf("warning detail must mention %q; got:\n%s", want, detail)
		}
	}
}

// TestSkippedCloudIdentityProviderDiagnostics_Separate asserts the two causes
// are reported as two warnings. They ask for different next steps, and merging
// them would tell an operator to hand-write a provider that was merely
// unreadable.
func TestSkippedCloudIdentityProviderDiagnostics_Separate(t *testing.T) {
	diags := skippedCloudIdentityProviderDiagnostics(
		[]skippedCloudIdentityProvider{{id: "g1", name: "G", reason: skipUnrepresentableGoogle}},
		[]skippedCloudIdentityProvider{{id: "e1", name: "E", reason: skipUnreadable, err: errors.New("boom")}},
	)
	if len(diags) != 2 {
		t.Fatalf("expected two warnings; got %d: %v", len(diags), diags)
	}
}

// TestSkippedCloudIdentityProviderDiagnostics_None guards the quiet path: a run
// that dropped nothing must add no diagnostics, so the caller can distinguish
// an empty registry from a fully skipped one.
func TestSkippedCloudIdentityProviderDiagnostics_None(t *testing.T) {
	if diags := skippedCloudIdentityProviderDiagnostics(nil, nil); len(diags) != 0 {
		t.Errorf("expected no diagnostics; got %v", diags)
	}
}
