// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldap_server

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// Wire enum values for the Jamf ProClassic /ldapservers endpoint, captured by
// the 2026-05-31 wire probe (see spike/LDAP_SERVER_SPIKE.md). The OneOf
// validators built from these block values the server silently normalises —
// unknown server_type falls back to "Active Directory", and referral_response
// is lower-cased — so divergent input can't reach apply.
const (
	// server_type. UI "Directory Service": "Microsoft's Active Directory" →
	// Active Directory, "Apple's Open Directory" → Open Directory, "Novell's
	// eDirectory" → eDirectory, "Configure Manually" → Custom.
	serverTypeActiveDirectory = proclassic.LdapServerConnectionServerTypeActiveDirectory
	serverTypeOpenDirectory   = proclassic.LdapServerConnectionServerTypeOpenDirectory
	serverTypeEDirectory      = proclassic.LdapServerConnectionServerTypeEDirectory
	serverTypeCustom          = proclassic.LdapServerConnectionServerTypeCustom

	// authentication_type. Mixed case is load-bearing: none/simple are
	// lower-case but CRAM-MD5/DIGEST-MD5 are upper-case on the wire — which is
	// exactly why these alias rather than restate.
	authTypeNone      = proclassic.LdapServerConnectionAuthenticationTypeNone
	authTypeSimple    = proclassic.LdapServerConnectionAuthenticationTypeSimple
	authTypeCRAMMD5   = proclassic.LdapServerConnectionAuthenticationTypeCramMd5
	authTypeDigestMD5 = proclassic.LdapServerConnectionAuthenticationTypeDigestMd5

	// referral_response. The empty string means "Use default from LDAP
	// service"; it is the absence of a choice rather than a member of
	// proclassic.LdapServerConnectionReferralResponse, which generates only the
	// two real values.
	referralDefault = ""
	referralFollow  = proclassic.LdapServerConnectionReferralResponseFollow
	referralIgnore  = proclassic.LdapServerConnectionReferralResponseIgnore

	// map_object_class_to_any_or_all. UI "Object Class Limitation". The classic
	// spec declares this vocabulary once per mapping block (user, user-group,
	// user-group-membership) with identical members; one pair serves all three,
	// which TestMappingVocabulariesAgree pins.
	objectClassAny = proclassic.LdapServerMappingsForUsersUserMappingsMapObjectClassToAnyOrAllAny
	objectClassAll = proclassic.LdapServerMappingsForUsersUserMappingsMapObjectClassToAnyOrAllAll

	// search_scope. Declared once per mapping block like the above; note the
	// Pro JSON vocabulary spells these ALL_SUBTREES / FIRST_LEVEL_ONLY, so the
	// classic constants are the only correct ones here.
	searchScopeAllSubtrees    = proclassic.LdapServerMappingsForUsersUserMappingsSearchScopeAllSubtrees
	searchScopeFirstLevelOnly = proclassic.LdapServerMappingsForUsersUserMappingsSearchScopeFirstLevelOnly

	// user_group_membership_stored_in. UI "Membership Location" dropdown shows
	// Group Object / User Object / Other, but only these two values round-trip
	// on the wire — sending "Other" is silently coerced to "group object"
	// (wire-probe 2026-05-31). "Other" is a UI-only display state for a config
	// that doesn't match either template; its extra fields (object_classes,
	// search_base, username_mapping, group_id_mapping) are plain optional
	// attributes here and round-trip regardless of membership_location.
	membershipGroupObject = proclassic.LdapServerMappingsForUsersUserGroupMembershipMappingsUserGroupMembershipStoredInGroupObject
	membershipUserObject  = proclassic.LdapServerMappingsForUsersUserGroupMembershipMappingsUserGroupMembershipStoredInUserObject
)

// Alphabetised OneOf value lists for diff-stable schema docs.
var (
	allServerTypes         = []string{serverTypeActiveDirectory, serverTypeCustom, serverTypeEDirectory, serverTypeOpenDirectory}
	allAuthenticationTypes = []string{authTypeCRAMMD5, authTypeDigestMD5, authTypeNone, authTypeSimple}
	allReferralResponses   = []string{referralDefault, referralFollow, referralIgnore}
	allObjectClassLimits   = []string{objectClassAll, objectClassAny}
	allSearchScopes        = []string{searchScopeAllSubtrees, searchScopeFirstLevelOnly}
	allMembershipLocations = []string{membershipGroupObject, membershipUserObject}
)

// preserveOnOmitString, preserveOnOmitBool, preserveOnOmitInteger,
// preserveOnOmitEnum and preserveOnOmitBlock are the house sentences every
// preserve-on-omit description ends with, per STYLE_GUIDE.md §Full-replace
// endpoints & shared backing stores ("Description convention"). The classic
// /ldapservers update merges field by field (see crud.go), so omitting an
// attribute — or a whole optional block — leaves the stored Jamf Pro value
// alone. The wording differs by type because only a string has a blank that
// clears.
const (
	preserveOnOmitString  = "Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."
	preserveOnOmitBool    = "Omit to leave the current value untouched; set `true`/`false` to change it."
	preserveOnOmitInteger = "Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it."
	preserveOnOmitEnum    = "Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it."
	preserveOnOmitBlock   = "Omit the block to leave any existing values untouched (they are not cleared on update)."
)

// optString is an Optional+Computed schema.StringAttribute with the canonical
// UseStateForUnknown plan modifier and preserveOnOmitString appended to the
// description. Used for connection / mapping fields the Jamf Pro server
// populates with a default when omitted: Optional+Computed keeps the server
// value out of the diff, and UseStateForUnknown keeps the prior state value
// across refreshes so omitted fields do not flap.
//
// Consequence (documented per advisor): because omitted fields fall back to
// the prior state value, deleting an HCL line for a mapping field does not
// clear it on the server — consistent with Classic's partial-merge PUT, and
// what the appended sentence tells the user.
func optString(desc string) schema.StringAttribute {
	return optStringPreserving(desc, preserveOnOmitString)
}

// optStringPreserving is optString with the closing preserve-on-omit sentence
// chosen by the caller, for a field whose blank means something other than
// "clear it".
func optStringPreserving(desc, preserve string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc + " " + preserve,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseNonNullStateForUnknown(),
		},
	}
}

// optStringOneOf is optString constrained to a fixed set of wire values, and
// closing on preserveOnOmitEnum rather than the string sentence — an enum has
// no blank to clear with. The OneOf blocks server-normalised input from
// reaching apply (which would throw "inconsistent result after apply" because
// the planned value is known).
func optStringOneOf(desc string, vals []string) schema.StringAttribute {
	return optStringOneOfPreserving(desc, vals, preserveOnOmitEnum)
}

// optStringOneOfPreserving is optStringOneOf with a caller-chosen
// preserve-on-omit sentence, for an enum that does accept the blank as one of
// its values (referral_response).
func optStringOneOfPreserving(desc string, vals []string, preserve string) schema.StringAttribute {
	a := optStringPreserving(desc, preserve)
	a.Validators = []validator.String{stringvalidator.OneOf(vals...)}
	return a
}

// reqString is a Required schema.StringAttribute with a non-empty constraint.
func reqString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Required:            true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
	}
}

// reqStringOneOf is a Required schema.StringAttribute constrained to a fixed
// set of wire values.
func reqStringOneOf(desc string, vals []string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Required:            true,
		Validators: []validator.String{
			stringvalidator.OneOf(vals...),
		},
	}
}

// optBool is the bool counterpart of optString, closing on
// preserveOnOmitBool.
func optBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc + " " + preserveOnOmitBool,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseNonNullStateForUnknown(),
		},
	}
}

// optInt64 is the int64 counterpart of optString, closing on
// preserveOnOmitInteger.
func optInt64(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: desc + " " + preserveOnOmitInteger,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Int64{
			int64planmodifier.UseNonNullStateForUnknown(),
		},
	}
}

// computedString is a Computed-only schema.StringAttribute for server-managed
// echoes the user never sets (e.g. certificates_used).
func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseNonNullStateForUnknown(),
		},
	}
}

// computedBool is the bool counterpart of computedString.
func computedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseNonNullStateForUnknown(),
		},
	}
}

// computedInt64 is the int64 counterpart of computedString.
func computedInt64(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: desc,
		Computed:            true,
		PlanModifiers: []planmodifier.Int64{
			int64planmodifier.UseNonNullStateForUnknown(),
		},
	}
}

// accountWoVersion returns the connection.account.password_wo_version value
// from a model, or a null Int64 when there is no account block.
func accountWoVersion(m LdapServerResourceModel) types.Int64 {
	if m.Connection == nil || m.Connection.Account == nil {
		return types.Int64Null()
	}
	return m.Connection.Account.PasswordWoVersion
}
