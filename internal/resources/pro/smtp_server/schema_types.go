// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smtp_server

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// Authentication-type discriminator values. Kept in one place so the schema
// OneOf validator, the cross-field ConfigValidator, the input-builder dispatch,
// and the state assigner share a single source of truth. These are both the
// Terraform-facing values and the Jamf Pro wire values (`authenticationType`).
const (
	authNone       = pro.SmtpServerV2AuthenticationTypeNone
	authBasic      = pro.SmtpServerV2AuthenticationTypeBasic
	authGraphAPI   = pro.SmtpServerV2AuthenticationTypeGraphApi
	authGoogleMail = pro.SmtpServerV2AuthenticationTypeGoogleMail
)

// authenticationTypes is the full discriminator set, for the OneOf validator.
var authenticationTypes = pro.SmtpServerV2AuthenticationTypeValues()

// encryptionTypes is the SMTP connection encryption enum (wire values). The
// Jamf Pro admin UI labels these None / SSL / TLSv1.3 / TLSv1.2 / TLSv1.1 /
// TLSv1 respectively; the resource uses the wire values verbatim.
var encryptionTypes = pro.SmtpConnectionSettingsEncryptionTypeValues()

// The read-only OAuth-grant status enum returned in
// google_mail_credentials.authentications[].status is FAILED / UNAUTHENTICATED /
// AUTHENTICATED. The field is Computed, so no validator consumes it.

// smtpServerTimeoutAttributeTypes defines the timeout attribute types for the
// resource operations.
var smtpServerTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// googleAuthenticationAttrTypes is the object type of one entry in the read-only
// google_mail_credentials.authentications Computed list.
var googleAuthenticationAttrTypes = map[string]attr.Type{
	"email_address": types.StringType,
	"status":        types.StringType,
}
