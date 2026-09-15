// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// vendorJamfPro is the only UEM vendor this resource supports.
//
// Jamf Security Cloud connects to ten vendors, but almost everything below this
// line is vendor-specific: each device-field mapping is a separate server-side
// enum whose members depend on the vendor, and the group identifier format
// differs too. Accepting a vendor whose vocabularies have not been established
// would mean validating an Intune group identifier against Jamf Pro's rule and
// rejecting valid input at plan time — worse than declining the vendor outright.
// Widening is additive when another vendor's vocabularies are established.
//
// Keyed on the create envelope's own discriminator, which is the only vocabulary
// that enumerates every vendor. It was once contrasted here with a generic
// ConnectorCreateRequestVendor enum that omitted JAMF_PRO; that type no longer
// exists — the spec completed the per-vendor create split and gave all ten
// vendors their own request schema — so the envelope is now the only candidate
// rather than the better of two.
const vendorJamfPro = securitycloud.ConnectorCreateRequestBodyVendorJamfPro

// authStrategyPlatformTenant and authStrategyOAuth are the two ways a Jamf Pro
// connector authenticates.
//
// The distinction is which of them the caller has to do work for.
// authStrategyPlatformTenant names the target by its platform tenant identifier
// and Jamf Security Cloud provisions its own credentials on that tenant;
// authStrategyOAuth takes credentials the operator created there by hand.
//
// Neither value survives the write: a connector created either way reports
// JAMF_PRO_OAUTH afterwards, because that is the steady state provisioning
// leaves behind. The strategy is therefore derived from which configuration
// block is present and never round-tripped through state — see the package doc
// comment.
const (
	authStrategyPlatformTenant = securitycloud.JamfProConnectorCreateRequestAuthStrategyM2m
	authStrategyOAuth          = securitycloud.JamfProConnectorCreateRequestAuthStrategyJamfProOauth
)

// uemAutoDeleteBehaviourToWire maps this resource's auto-delete values to the ones
// Jamf Security Cloud stores.
//
// All three are translated because the wire spelling describes the feature's
// internal state and the admin UI describes what happens to the devices, which
// is what an operator is choosing between. `DISABLED` is the clearest case: it
// reads as "auto-deletion is off" when the setting it expresses is "keep deleted
// or retired devices in Jamf Security Cloud".
//
// Keyed on the SDK's generated constants, so a renamed value fails the build
// rather than silently sending a string the server rejects.
var uemAutoDeleteBehaviourToWire = map[string]string{
	"keep_deleted_or_retired":     securitycloud.SyncSettingsAutoDeviceDeletionDisabled,
	"remove_deleted_or_retired":   securitycloud.SyncSettingsAutoDeviceDeletionDeletedOrRetired,
	"remove_deleted_or_unmanaged": securitycloud.SyncSettingsAutoDeviceDeletionUnmanaged,
}

// uemAutoDeleteBehaviourFromWire is the reverse of uemAutoDeleteBehaviourToWire, built
// from it so the two cannot disagree.
var uemAutoDeleteBehaviourFromWire = reverseMapping(uemAutoDeleteBehaviourToWire)

// uemAutoDeleteBehaviourValues returns the accepted auto-delete values in a stable
// order, for the plan-time validator and the rendered documentation.
func uemAutoDeleteBehaviourValues() []string {
	return sortedMapKeys(uemAutoDeleteBehaviourToWire)
}

// The device-field mapping vocabularies, per key, for the Jamf Pro vendor.
//
// These are restated rather than taken from the SDK, which is the exception the
// Security Cloud enum rule allows for. Each key is a distinct server-side enum
// whose members depend on the connector's vendor, so the spec types all five as
// plain strings and the generator has nothing to enumerate. The one helper the
// SDK does generate, EmailMappingTypeValues(), spans every vendor and is a
// superset here: it includes EXTERNAL_USER_ID and CUSTOM, which a Jamf Pro
// connector rejects.
//
// Every set below was read off the 422 the server returns for an unknown value,
// which enumerates the accepted members (wire-verified 2026-08-28, EU).
//
// The values keep their wire spelling. The admin UI's dropdowns show labels for
// the five defaults — "Device name", "User name", "External user ID", "Phone
// number", "Email" — which the wire spelling already matches closely enough to
// read, and no screen was available showing the other nine (IMEI, MDM_ID,
// SERIAL_NUMBER, FIRST_LAST_NAME, NO_CHANGE, NO_PHONE_NUMBER, FIRST_NAME,
// LAST_NAME, NAME). Translating five and inventing nine would put made-up labels
// in a user-facing enum, which is worse than a wire spelling that reads as a
// field name already.
var (
	deviceNameMappingValues = []string{
		"DEVICE_NAME",
		"IMEI",
		"MDM_ID",
		"PHONE_NUMBER",
		"SERIAL_NUMBER",
		"USER_NAME",
	}

	userNameMappingValues = []string{
		"DEVICE_NAME",
		"EMAIL_ADDRESS",
		"FIRST_LAST_NAME",
		"MDM_ID",
		"NO_CHANGE",
		"SERIAL_NUMBER",
		"USER_NAME",
	}

	userIDMappingValues = []string{
		"EMAIL_ADDRESS",
		"EXTERNAL_USER_ID",
		"FIRST_LAST_NAME",
		"MDM_ID",
		"NO_CHANGE",
		"USER_NAME",
	}

	phoneNumberMappingValues = []string{
		"NO_PHONE_NUMBER",
		"PHONE_NUMBER",
	}

	// userEmailMappingTypeValues keys on the SDK's generated EmailMappingType
	// constants where they exist, so a rename upstream breaks the build. The two
	// members of that enum a Jamf Pro connector refuses are simply absent.
	userEmailMappingTypeValues = []string{
		securitycloud.EmailMappingTypeDeviceName,
		securitycloud.EmailMappingTypeEmailAddress,
		securitycloud.EmailMappingTypeFirstName,
		securitycloud.EmailMappingTypeImei,
		securitycloud.EmailMappingTypeLastName,
		securitycloud.EmailMappingTypeMDMID,
		securitycloud.EmailMappingTypeName,
		securitycloud.EmailMappingTypeSerialNumber,
	}
)

// The Jamf Pro defaults for each device-field mapping, as the server applies them
// when a write omits the mapping. Recorded so the documentation can name them and
// so a test can pin that the documented default is the one the server chose
// (wire-verified 2026-08-28).
//
// Four are literals and the fifth is an SDK constant, which looks inconsistent and
// is not. Only the email mapping has a generated enum; the other four keys are
// separate server-side enums the spec types as plain strings. The SDK does happen to
// carry an EmailMappingTypeDeviceName whose value is also "DEVICE_NAME", but it
// belongs to the email enum — borrowing it here would tie this default to an
// unrelated vocabulary that merely shares a spelling today.
const (
	defaultDeviceNameMapping    = "DEVICE_NAME"
	defaultUserNameMapping      = "USER_NAME"
	defaultUserIDMapping        = "EXTERNAL_USER_ID"
	defaultPhoneNumberMapping   = "PHONE_NUMBER"
	defaultUserEmailMappingType = securitycloud.EmailMappingTypeEmailAddress
)
