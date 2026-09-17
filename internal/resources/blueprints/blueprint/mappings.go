// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import "github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"

// Constants for blueprint deployment states. The SDK also generates
// OUT_OF_DATE, which nothing here compares against.
const (
	blueprintDeploymentStateDeployed    = blueprints.DeploymentStateStateDeployed
	blueprintDeploymentStateNotDeployed = blueprints.DeploymentStateStateNotDeployed
)

// appleDeclarationsIdentifier is the wire identifier of "All Declarations" in the Jamf Pro
// blueprint editor, which the apple_declarations attribute is named for. An identifier is API
// plumbing and must not reach user-facing text (see STYLE_GUIDE §Attribute names mirror the Jamf
// Pro admin UI).
const appleDeclarationsIdentifier = "com.jamf.ddm-strict"

// legacyConfigProfileIdentifier is the wire identifier of the component a block's legacy payloads
// are folded into, which the legacy_payloads attribute writes. Like appleDeclarationsIdentifier it
// is named here because no components/ converter owns it — the component is assembled in this
// package — and it must not reach user-facing text.
const legacyConfigProfileIdentifier = "com.jamf.ddm-configuration-profile"

// mcxPayloadType is Apple's payload type for the managed preferences envelope the Jamf Pro profile
// editor calls "Application & Custom Settings", and mcxPreferenceDomainsKey is Apple's spelling of
// the dictionary its preference domains sit under.
//
// It is the one payload type a component block may carry more than once, because a payload holds a
// single preference domain: wire probing on macOS 26.6 on 2026-09-17 found a payload storing three
// domains applied exactly one of them and dropped the rest, with the deploy reporting SUCCEEDED and
// com.apple.ManagedClient logging nothing. Which domain survived was not predictable — a trio
// stored as Safari, SoftwareUpdate, Terminal applied SoftwareUpdate, and a quad stored as
// Accessibility, Safari, Terminal, dock applied dock, so neither first nor last fits both, and Jamf
// re-sorts the keys into ASCII order anyway. The same domains split one per payload all applied,
// which is also how the Jamf Pro editor builds a profile.
const (
	mcxPayloadType          = "com.apple.ManagedClient.preferences"
	mcxPreferenceDomainsKey = "PayloadContent"
)

// stronglyTypedComponentIdentifiers lists all component identifiers that have strongly-typed representations.
var stronglyTypedComponentIdentifiers = map[string]struct{}{
	"com.jamf.ai-governance":                   {},
	"com.jamf.ddm.audio-accessory-settings":    {},
	"com.jamf.ddm.custom-declarations":         {},
	"com.jamf.ddm.disk-management":             {},
	"com.jamf.ddm.math-settings":               {},
	"com.jamf.ddm.passcode-settings":           {},
	"com.jamf.ddm.safari-bookmarks":            {},
	"com.jamf.ddm.safari-extensions":           {},
	"com.jamf.ddm.safari-settings":             {},
	"com.jamf.ddm.service-background-tasks":    {},
	"com.jamf.ddm.service-configuration-files": {},
	"com.jamf.ddm.sw-updates":                  {},
	"com.jamf.ddm.software-update-settings":    {},
	"com.jamf.ddm-configuration-profile":       {},
	"com.jamf.ddm-strict":                      {},
}

// blockOnlyComponentIdentifiers lists the strongly-typed components the provider offers only inside
// component_blocks. The flat top-level component style is deprecated and removed on or after
// 2026-10-22, so no component added after it was deprecated gains a flat attribute — which leaves a
// flat-mode read with nowhere to put one. Such a component is kept in raw_component rather than
// dropped, because a flat-mode apply rewrites every step: a component missing from state is deleted
// from the blueprint with no plan diff showing the deletion. Every entry must also appear in
// stronglyTypedComponentIdentifiers, and state_builders_test.go pins that a typed component with no
// BlueprintResourceModel field is registered here.
var blockOnlyComponentIdentifiers = map[string]struct{}{
	"com.jamf.ai-governance": {},
	"com.jamf.ddm-strict":    {},
}

// legacyPayloadSettingsBehaviour documents how Jamf treats the settings written for a legacy payload,
// appended to every legacy-payload schema description. It deliberately covers only what an author
// cannot learn from a diagnostic: what the provider absorbs silently, and the one case it cannot fix
// (an import cannot recover a redacted value). The rules the plan-time schema check reports — an
// unrecognised or miscased key, a wrong value type, a missing required key, an out-of-range integer —
// are named there, with the offending path and Apple's spelling, so they are summarised here rather
// than enumerated. See internal/common/appleprofiles.
const legacyPayloadSettingsBehaviour = "The platform validates each payload against Apple's payload keys for its `payload_type`, " +
	"and the provider checks the same rules during `plan`, so an unrecognised or miscased key, a wrong value type, " +
	"or a missing required key is reported before an apply rather than failing one. " +
	"Each of those is an **error**: Jamf drops a key it does not recognise while reporting success, so a payload " +
	"carrying one never applies. " +
	"A custom settings payload (`com.apple.ManagedClient.preferences`) sets exactly one preference domain under " +
	"`PayloadContent`, and the provider refuses any other count during `plan`. A Mac applies one of several domains and " +
	"drops the rest. The platform discards an empty dictionary. Write one payload per domain, as the Jamf Pro " +
	"profile editor does; a block may carry as many custom settings payloads as it has domains. " +
	"To skip the checks, move **every** legacy payload in the same block to a " +
	"single `raw_component` with identifier `com.jamf.ddm-configuration-profile`. The platform stores a " +
	"block's legacy payloads as one component, so move them all or leave them all here. A `raw_component` sends a " +
	"multi-domain payload unchanged, so a Mac still drops all but one domain and the block's payload identifiers " +
	"are reissued on every write. Split the domains rather than moving them. " +
	"Two behaviours are absorbed for you instead: a key set to `null` is discarded by Jamf and tolerated here, so nulls " +
	"can stay in configuration; and Apple's common payload metadata (`payloadDisplayName`, `payloadOrganization`, " +
	"`payloadVersion`) is stamped onto every payload and hidden unless you set it yourself. " +
	"Values the platform treats as credentials (a Wi-Fi `Password`, and `EAPClientConfiguration`'s `UserName`, `UserPassword` " +
	"and `OuterIdentity`) are returned redacted, and the provider keeps what you wrote so the plan still settles. " +
	"An imported blueprint carries the redaction, because the real value cannot be read back."
