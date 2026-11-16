// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package payloadhelpers

import (
	"strings"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/plisthelpers"
)

// ProfilePlatform selects which storage-category table applies. The category is
// per-platform, not global: Jamf Pro stores com.apple.ManagedClient.preferences
// faithfully in a macOS profile but verbatim in a mobile-device profile, and
// com.apple.applicationaccess the other way round. One shared table would get
// one platform wrong. testdata/reserved_character_corpus.mobileconfig sits in
// both resources' fixtures with identical content and opposite expected
// verdicts, and that pair is the regression guard.
type ProfilePlatform int

const (
	// PlatformMacOS — jamfplatform_pro_macos_configuration_profile.
	PlatformMacOS ProfilePlatform = iota
	// PlatformMobileDevice — jamfplatform_pro_mobile_device_configuration_profile.
	PlatformMobileDevice
)

// Storage categories, wire-probed against a live Jamf Pro tenant (harness:
// create a profile carrying the type with "&", "<", ">", LF, TAB and a CR
// reference in a string value; read back; compare values).
//
// Two categories, per (platform, PayloadType):
//
//   - re-render — Jamf Pro parses the fragment and re-serialises it. Every
//     probe value survives: "&", "<", LF and TAB all round-trip. Unknown keys
//     are preserved, so this is not schema-driven filtering.
//   - verbatim — Jamf Pro stores the submitted bytes as received. Because the
//     Classic API requires "&" pre-escaped once (the server entity-decodes the
//     submission once to validate, so a bare "&" is rejected outright — see the
//     SDK's PayloadsXMLText contract), a verbatim slot keeps that extra layer:
//     a value of `&` comes back as `&amp;`, `<` as `&lt;`. LF and TAB are
//     deleted outright. ">" and CR survive. This is Jamf product issue PI-827
//     and no client can work around it.
//
// Only re-render types are listed. Anything absent — including every payload
// type not probed and every string slot outside the PayloadContent array (the
// top-level dict, e.g. a ConsentText sub-dictionary) — is treated as verbatim.
// Deny-by-default is deliberate: a wrong "verbatim" guess costs one refused
// import, which is loud and fixed by an entry here, while a wrong "re-render"
// guess lets a profile through to be silently corrupted by its first update,
// irreversibly. Never guess in the second direction.
//
// There is no override, by design. A profile Jamf Pro cannot store faithfully
// cannot be managed safely, and a provider-wide switch would reinstate the
// corruption. A misclassification is corrected here for everyone, and the
// diagnostic asks the operator to report the PayloadType.
//
// Adding an entry requires a wire probe, not inference — see
// storage_category_probe_test.go (build tag `payload_probe`). Roughly a quarter
// of the types first probed turned out to be re-render (servicemanagement,
// extensiblesso, webcontent-filter), so leaving a type unprobed refuses imports
// that would have been safe. A type whose synthetic payloads Jamf Pro rejects
// on schema grounds, as webcontent-filter did, is probed from a real payload
// with TestProbeStorageCategoriesFromRealPayload.
//
// Probed and found verbatim on the platforms tried, so not worth re-probing:
// com.apple.loginwindow, com.apple.MCX,
// com.apple.TCC.configuration-profile-policy, com.apple.vpn.managed,
// com.apple.wifi.managed, com.apple.dock, com.apple.screensaver,
// com.apple.SetupAssistant.managed, com.apple.systempreferences,
// com.apple.finder, com.apple.appstore, com.apple.dashboard,
// com.apple.system-extension-policy,
// com.apple.syspolicy.kernel-extension-policy and, on mobile devices,
// com.apple.webClip.managed. Unprobed on mobile devices:
// com.apple.systempolicy.control, com.apple.security.firewall,
// com.apple.SubmitDiagInfo, com.apple.servicemanagement,
// com.apple.extensiblesso. Unprobed on macOS:
// com.apple.shareddeviceconfiguration.
//
// The table answers the escaping question only. A re-render type can still
// gain or lose keys on ingest (a web clip gains Icon and data_files and loses
// keys Jamf Pro does not recognise), which is a separate matter the mask
// handles.
var faithfulPayloadTypes = map[ProfilePlatform]map[string]struct{}{
	PlatformMacOS: {
		"com.apple.ManagedClient.preferences":   {}, // "Application & Custom Settings"
		"com.apple.notificationsettings":        {},
		"com.apple.systempolicy.control":        {},
		"com.apple.security.firewall":           {},
		"com.apple.mobiledevice.passwordpolicy": {},
		"com.apple.SubmitDiagInfo":              {},
		"com.apple.servicemanagement":           {},
		"com.apple.extensiblesso":               {},
		"com.apple.webcontent-filter":           {},
	},
	PlatformMobileDevice: {
		"com.apple.notificationsettings":        {},
		"com.apple.applicationaccess":           {},
		"com.apple.shareddeviceconfiguration":   {},
		"com.apple.mobiledevice.passwordpolicy": {},
		"com.apple.webcontent-filter":           {},
	},
}

// PayloadImportRisk reports whether Jamf Pro would mangle this payload if
// Terraform wrote it back, and if so returns diagnostic detail naming each
// affected value. It performs no API calls and writes nothing: the prediction is
// derived from the payload text alone by applying the wire law above, then
// diffing the result through diffPayloadTrees, the comparison the post-write
// fidelity checks use. The prediction must never be judged by a separate
// predicate.
//
// Callers gate on import only. Ordinary refresh must not consult this: a profile
// corrupted outside Terraform, or already under management, has to keep
// refreshing so drift stays visible and the resource can still be removed.
//
// ok=false means the payload did not parse as a plist, in which case the caller
// should let the import through — an unparseable payload is a different problem
// and the post-write checks still backstop it.
func PayloadImportRisk(payload []byte, platform ProfilePlatform) (detail string, lossy, ok bool) {
	tree, _, err := plisthelpers.ParsePlist(payload)
	if err != nil {
		return "", false, false
	}
	predicted := predictStoredTree(tree, platform, false)
	findings := diffPayloadTrees(tree, predicted)
	if len(findings) == 0 {
		return "", false, true
	}
	return formatFidelityFindings(findings, FidelityPhaseImport), true, true
}

// predictStoredTree deep-clones a parsed payload, rewriting every string in a
// verbatim slot into the form Jamf Pro would store. verbatim is threaded down
// the walk: it starts false at the root (the top level is decided per-slot
// below) and is fixed for a whole PayloadContent entry by that entry's
// PayloadType.
func predictStoredTree(v any, platform ProfilePlatform, verbatim bool) map[string]any {
	out, _ := predictValue(v, platform, verbatim, true).(map[string]any)
	return out
}

// predictValue applies the transform recursively. topLevel marks the root dict,
// whose PayloadContent array is the only place PayloadType switches the
// category; every other string at the root is a verbatim slot.
func predictValue(v any, platform ProfilePlatform, verbatim, topLevel bool) any {
	switch t := v.(type) {
	case string:
		if !verbatim {
			return t
		}
		return applyVerbatimStorage(t)

	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = predictValue(item, platform, verbatim, false)
		}
		return out

	case map[string]any:
		entryVerbatim := verbatim
		if !topLevel {
			// A dict carrying a PayloadType is a payload fragment: its category is
			// decided here and inherited by everything beneath it.
			if ptype, isPayload := t["PayloadType"].(string); isPayload {
				entryVerbatim = !isFaithfulPayloadType(platform, ptype)
			}
		}
		out := make(map[string]any, len(t))
		for k, val := range t {
			childVerbatim := entryVerbatim
			childTopLevel := false
			if topLevel {
				// Strings directly on the root dict live in a verbatim slot; the
				// PayloadContent array's entries decide for themselves.
				childVerbatim = k != "PayloadContent"
			}
			// A key in a verbatim slot is rewritten just like a value, which surfaces
			// as the authored key going missing from the predicted tree.
			outKey := k
			if childVerbatim {
				outKey = applyVerbatimStorage(k)
			}
			out[outKey] = predictValue(val, platform, childVerbatim, childTopLevel)
		}
		return out

	default:
		return v
	}
}

// applyVerbatimStorage is the verbatim-slot transform: one extra entity layer
// around "&" and "<" (PI-827), line feeds and tabs deleted. ">" , CR, U+2028,
// U+2029 and U+0085 are left alone — all wire-verified to survive.
func applyVerbatimStorage(s string) string {
	if !strings.ContainsAny(s, "&<\n\t") {
		return s
	}
	return verbatimStorageReplacer.Replace(s)
}

var verbatimStorageReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	"\n", "",
	"\t", "",
)

func isFaithfulPayloadType(platform ProfilePlatform, ptype string) bool {
	_, faithful := faithfulPayloadTypes[platform][ptype]
	return faithful
}
