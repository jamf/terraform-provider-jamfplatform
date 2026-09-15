// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// boolOrCurrent emits the plan value when known — declared, or carried in from
// prior state by UseStateForUnknown — and otherwise the value the tenant already
// holds. It is what makes an omitted toggle preserve rather than reset on the
// first apply; see buildSsoSettingsInput on the merge base.
func boolOrCurrent(v types.Bool, current bool) bool {
	if p := helpers.OptionalBoolPointer(v); p != nil {
		return *p
	}
	return current
}

// boolPtrOrCurrent is boolOrCurrent for a nullable bool, keeping nil when
// neither the plan nor the tenant has a value so a caller can supply its own
// default.
func boolPtrOrCurrent(v types.Bool, current *bool) *bool {
	if p := helpers.OptionalBoolPointer(v); p != nil {
		return p
	}
	return current
}

// stringPtrOrCurrent emits the plan value when known and otherwise the tenant's
// stored one. An explicitly configured "" is a known value, so clearing a field
// still works.
func stringPtrOrCurrent(v types.String, current *string) *string {
	if p := helpers.OptionalStringPointer(v); p != nil {
		return p
	}
	return current
}

// intPtrOrCurrent is stringPtrOrCurrent for a nullable int.
func intPtrOrCurrent(v types.Int64, current *int) *int {
	if p := helpers.OptionalInt64Pointer(v); p != nil {
		return p
	}
	return current
}

// enrollmentManagementHint reads the management hint off a possibly-absent
// enrollment SSO block, so the merge base can be consulted without a nil check
// at the call site.
func enrollmentManagementHint(current *pro.EnrollmentSsoConfig) *string {
	if current == nil {
		return nil
	}
	return current.ManagementHint
}

// federationMetadataFileOrCurrent decodes the base64 IdP metadata the
// configuration supplied, falling back to the metadata the tenant already
// stores. Both are the FILE-mode payload; URL mode clears it instead.
func federationMetadataFileOrCurrent(v types.String, current *[]byte) (*[]byte, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !isStringConfigured(v) {
		return current, diags
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v.ValueString()))
	if err != nil {
		diags.AddAttributeError(
			path.Root("saml_settings").AtName("federation_metadata_file"),
			"Invalid federation_metadata_file base64",
			"The supplied federation_metadata_file is not valid RFC 4648 base64: "+helpers.APIErrorDetail(err),
		)
		return nil, diags
	}
	return &decoded, diags
}

// buildSsoSettingsInput converts the Terraform plan model into a /v3/sso PUT
// payload, merged over the settings the tenant already holds.
//
// # The merge base
//
// current is the live settings read, and every field the configuration does not
// declare takes its value from there. The PUT is a genuine full replacement:
// wire-probed 2026-09-08 on oidcSettings.usernameAttributeClaimMapping, a key
// sent as null and a key left out of the body both reset the field to the
// server's default, and an empty string is rejected outright. So a body built
// from the plan alone resets every setting the configuration is silent about.
//
// UseStateForUnknown hides most of that on update — an omitted Optional+Computed
// attribute plans as its prior state value and is re-sent unchanged — but there
// is no prior state on the first apply, and this singleton always pre-exists.
// Create is therefore adoption, not creation, and needs the live settings as its
// base so the first apply keeps what it was not asked to change (#405).
//
// Update reads them too, which is where this departs from re_enrollment_settings
// and the STYLE_GUIDE pattern it follows ("Singletons: GET-on-create to adopt,
// not clobber"). That pattern rests on every attribute being Optional+Computed;
// here the enrollment SSO block and the SAML metadata file are Optional-only, so
// they plan as null when the configuration omits them and null resets. The base
// never overrides a plan value, so reading it on update costs one request and
// changes nothing else: a declared field still wins, and "" still clears.
//
// A nil current is therefore only the no-merge-base case the unit tests use, and
// it reproduces the pre-fix body exactly.
//
// Three load-bearing transforms sit on top of the merge:
//
//  1. OidcSettings is a value type on SsoSettingsV3 and its UserMapping field
//     has no omitempty, so the marshalled body always includes
//     `oidcSettings.userMapping`. Jamf Pro rejects an empty userMapping. In
//     pure SAML mode, when the user did not supply an oidc_settings block, the
//     tenant's stored block carries it — and a stub UserMapping="EMAIL" covers
//     a tenant that holds nothing. The value is harmless either way, since Jamf
//     ignores oidcSettings when configurationType=SAML.
//
//  2. metadata_source URL/FILE mutex is enforced on the wire regardless of
//     what the user wrote, and it runs after the merge so the mode the
//     configuration declares always wins: URL mode zeroes
//     federationMetadataFile and metadataFileName; FILE mode zeroes idpUrl.
//     metadataFileName must be null (not empty-string) in URL mode.
//
//  3. groupRdnKey must serialise as "" rather than null when SAML mode is
//     active — null is rejected (probed 2026-05-26), which is why this one
//     field cannot fall back to "unset" the way its siblings do. The merge base
//     is what lets an omitted group_rdn_key keep the tenant's token on the
//     first apply; only a tenant that holds none gets the explicit "".
func buildSsoSettingsInput(ctx context.Context, plan SsoSettingsResourceModel, current *pro.SsoSettingsV3) (*pro.SsoSettingsV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	base := current
	if base == nil {
		base = &pro.SsoSettingsV3{}
	}

	out := &pro.SsoSettingsV3{
		ConfigurationType:             plan.ConfigurationType.ValueString(),
		SsoEnabled:                    boolOrCurrent(plan.SsoEnabled, base.SsoEnabled),
		SsoBypassAllowed:              boolOrCurrent(plan.SsoBypassAllowed, base.SsoBypassAllowed),
		SsoForEnrollmentEnabled:       boolOrCurrent(plan.SsoForEnrollmentEnabled, base.SsoForEnrollmentEnabled),
		SsoForMacOsSelfServiceEnabled: boolOrCurrent(plan.SsoForMacOsSelfServiceEnabled, base.SsoForMacOsSelfServiceEnabled),
		EnrollmentSsoForAccountDrivenEnrollmentEnabled: boolOrCurrent(plan.EnrollmentSsoForAccountDrivenEnrollmentEnabled, base.EnrollmentSsoForAccountDrivenEnrollmentEnabled),
		GroupEnrollmentAccessEnabled:                   boolOrCurrent(plan.GroupEnrollmentAccessEnabled, base.GroupEnrollmentAccessEnabled),
		GroupEnrollmentAccessName:                      stringPtrOrCurrent(plan.GroupEnrollmentAccessName, base.GroupEnrollmentAccessName),
	}

	if plan.OidcSettings != nil {
		out.OidcSettings = pro.OidcSettings{
			UserMapping:                   plan.OidcSettings.UserMapping.ValueString(),
			JamfIDAuthenticationEnabled:   boolPtrOrCurrent(plan.OidcSettings.JamfIDAuthenticationEnabled, base.OidcSettings.JamfIDAuthenticationEnabled),
			UsernameAttributeClaimMapping: stringPtrOrCurrent(plan.OidcSettings.UsernameAttributeClaimMapping, base.OidcSettings.UsernameAttributeClaimMapping),
		}
	} else {
		out.OidcSettings = base.OidcSettings
		if out.OidcSettings.UserMapping == "" {
			out.OidcSettings.UserMapping = pro.OidcSettingsUserMappingEmail
		}
	}

	if plan.SamlSettings != nil {
		sp := plan.SamlSettings
		bs := base.SamlSettings

		tokenExpirationDisabled := boolPtrOrCurrent(sp.TokenExpirationDisabled, bs.TokenExpirationDisabled)
		if tokenExpirationDisabled == nil {
			t := true
			tokenExpirationDisabled = &t
		}
		userAttributeEnabled := boolPtrOrCurrent(sp.UserAttributeEnabled, bs.UserAttributeEnabled)
		if userAttributeEnabled == nil {
			f := false
			userAttributeEnabled = &f
		}
		out.SamlSettings = pro.SamlSettings{
			IdpProviderType:         stringPtrOrCurrent(sp.IdpProviderType, bs.IdpProviderType),
			OtherProviderTypeName:   stringPtrOrCurrent(sp.OtherProviderTypeName, bs.OtherProviderTypeName),
			EntityID:                stringPtrOrCurrent(sp.EntityID, bs.EntityID),
			MetadataSource:          stringPtrOrCurrent(sp.MetadataSource, bs.MetadataSource),
			SessionTimeout:          intPtrOrCurrent(sp.SessionTimeout, bs.SessionTimeout),
			TokenExpirationDisabled: tokenExpirationDisabled,
			UserMapping:             stringPtrOrCurrent(sp.UserMapping, bs.UserMapping),
			UserAttributeEnabled:    userAttributeEnabled,
			UserAttributeName:       stringPtrOrCurrent(sp.UserAttributeName, bs.UserAttributeName),
			GroupAttributeName:      stringPtrOrCurrent(sp.GroupAttributeName, bs.GroupAttributeName),
		}

		metadataSource := ""
		if !sp.MetadataSource.IsNull() && !sp.MetadataSource.IsUnknown() {
			metadataSource = sp.MetadataSource.ValueString()
		}
		switch metadataSource {
		case metadataSourceURL:
			out.SamlSettings.IdpURL = stringPtrOrCurrent(sp.IdpURL, bs.IdpURL)
			out.SamlSettings.MetadataFileName = nil
			out.SamlSettings.FederationMetadataFile = nil
		case metadataSourceFILE:
			out.SamlSettings.IdpURL = nil
			out.SamlSettings.MetadataFileName = stringPtrOrCurrent(sp.MetadataFileName, bs.MetadataFileName)
			file, d := federationMetadataFileOrCurrent(sp.FederationMetadataFile, bs.FederationMetadataFile)
			diags.Append(d...)
			if diags.HasError() {
				return nil, diags
			}
			out.SamlSettings.FederationMetadataFile = file
		default:
			out.SamlSettings.IdpURL = stringPtrOrCurrent(sp.IdpURL, bs.IdpURL)
			out.SamlSettings.MetadataFileName = stringPtrOrCurrent(sp.MetadataFileName, bs.MetadataFileName)
			file, d := federationMetadataFileOrCurrent(sp.FederationMetadataFile, bs.FederationMetadataFile)
			diags.Append(d...)
			if diags.HasError() {
				return nil, diags
			}
			out.SamlSettings.FederationMetadataFile = file
		}

		if key := stringPtrOrCurrent(sp.GroupRdnKey, bs.GroupRdnKey); key != nil {
			out.SamlSettings.GroupRdnKey = key
		} else {
			empty := ""
			out.SamlSettings.GroupRdnKey = &empty
		}
	} else {
		out.SamlSettings = base.SamlSettings
	}

	if plan.EnrollmentSsoConfig != nil {
		ec := &pro.EnrollmentSsoConfig{
			ManagementHint: stringPtrOrCurrent(plan.EnrollmentSsoConfig.ManagementHint, enrollmentManagementHint(base.EnrollmentSsoConfig)),
		}
		if !plan.EnrollmentSsoConfig.Hosts.IsNull() && !plan.EnrollmentSsoConfig.Hosts.IsUnknown() {
			hosts, d := helpers.SetToStringSlice(ctx, plan.EnrollmentSsoConfig.Hosts)
			diags.Append(d...)
			if diags.HasError() {
				return nil, diags
			}
			ec.Hosts = &hosts
		} else if base.EnrollmentSsoConfig != nil {
			ec.Hosts = base.EnrollmentSsoConfig.Hosts
		}
		out.EnrollmentSsoConfig = ec
	} else {
		out.EnrollmentSsoConfig = base.EnrollmentSsoConfig
	}

	return out, diags
}

// buildSsoCertificateInput converts the signing_certificate sub-block to the
// /v2/sso/cert PUT payload. Only valid when setup_type=UPLOADED.
//
// `key` is the alias lookup field; `keys[]` is informational and tolerated
// empty, so we omit it. `keystoreSetupType` is left nil — the upload shape
// is identified by the presence of `password` + `key`, not by an explicit
// setupType marker.
func buildSsoCertificateInput(plan signingCertificateModel) (*pro.SsoKeystore, diag.Diagnostics) {
	var diags diag.Diagnostics

	if plan.SetupType.ValueString() != setupTypeUploaded {
		diags.AddError(
			"Internal error: buildSsoCertificateInput called for non-UPLOADED setup_type",
			"This is a provider bug — the CRUD orchestrator should only call this builder when setup_type=UPLOADED.",
		)
		return nil, diags
	}

	keystoreBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(plan.KeystoreFile.ValueString()))
	if err != nil {
		diags.AddAttributeError(
			path.Root("signing_certificate").AtName("keystore_file"),
			"Invalid keystore_file base64",
			"The supplied keystore_file is not valid RFC 4648 base64: "+helpers.APIErrorDetail(err),
		)
		return nil, diags
	}

	return &pro.SsoKeystore{
		Key:              plan.Key.ValueString(),
		KeystoreFile:     keystoreBytes,
		KeystoreFileName: plan.KeystoreFileName.ValueString(),
		KeystorePassword: plan.KeystorePassword.ValueString(),
		Password:         plan.Password.ValueString(),
		Type:             plan.Type.ValueString(),
	}, diags
}
