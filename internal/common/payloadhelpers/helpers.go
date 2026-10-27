// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package payloadhelpers provides shared mobileconfig plist parsing,
// masking, and diff-suppression helpers used by all configuration profile
// resources (macOS and mobile device). All functions are pure — they operate
// on byte slices and maps and carry no resource-specific state.
package payloadhelpers

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"slices"
	"strings"

	"howett.net/plist"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/plisthelpers"
)

// Server-controlled keys skipped on both sides before diff comparison.
// Only keys Jamf Pro derives on every write/read regardless of input value
// land here.
//
// Keys Jamf Pro only *conditionally* defaults (e.g. `PayloadEnabled` when
// absent, `VendorConfig` only for webcontent-filter payloads) are
// intentionally **not** in this list. The intersection-compare in
// LenientEqualPlist drops asymmetric keys at comparison time, which means:
//
//   - user omits the key → only server side has it → intersection drops it → no spurious diff.
//   - user authors the key → both sides have it → compared → drift detected if the user later edits.
//
// `data_files` is in this list for a stronger reason than one-sidedness. It is
// Jamf Pro's handle for a web clip's stored icon, it appears only on the stored
// side, and the intersection compare would ignore it there — but the strict
// comparator behind `ThreeWayCompare` and `PayloadsStructurallyEqual` requires
// matching keysets and values, and those two compare *two server payloads*
// (the one stashed at apply against the one read back now) to tell an
// admin-side edit from a steady refresh. Wire-probed 2026-09-09 against Jamf
// Pro 11.3x: two consecutive reads of one unchanged profile return different
// `data_files` values, so without the mask every refresh of an icon-bearing
// profile reports drift that nobody caused, and the plan never settles
// (issue #418). Mobile-device profiles emit it; macOS profiles do not.
//
// `PayloadOrganization` is in this list — wire-confirmed 2026-05-26 that
// Jamf Pro Classic always overwrites the field with "JAMF Software" on
// both top-level and per-PayloadContent slots, regardless of the
// user-authored value. The previous "conditional default" assumption
// produced persistent payload diffs on every plan that authored an
// organization. Trade-off: a user-authored edit to PayloadOrganization
// will be silently dropped on the wire — but Jamf already drops it
// regardless, so masking the key matches reality. Drift detection on
// every other value the user authors is unaffected.
var (
	maskedTopLevelKeys = map[string]struct{}{
		"PayloadDisplayName":  {}, // server sets from classic <general><name> on every write
		"PayloadIdentifier":   {}, // server assigns a new lowercase UUID on create
		"PayloadUUID":         {}, // server assigns a new lowercase UUID on create
		"PayloadOrganization": {}, // server always overwrites with "JAMF Software"
		"PayloadDescription":  {}, // server always empties the top-level description
		"PayloadEnabled":      {}, // server always strips at top-level (Apple-spec is per-payload, not top-level)
	}
	maskedPayloadContentKeys = map[string]struct{}{
		"PayloadDisplayName":  {}, // server-defaulted per PayloadType
		"PayloadIdentifier":   {}, // server may assign on create
		"PayloadUUID":         {}, // server may assign on create; preserved on update by InjectTopLevelIdentifiers
		"PayloadOrganization": {}, // server always overwrites with "JAMF Software"
		"data_files":          {}, // handle for a web clip's stored icon, reissued on every read
	}
	// serverInjectedPayloadTypes are PayloadContent[i].PayloadType values Jamf
	// Pro inserts into the mobileconfig as a side-effect of a *different*
	// classic-API field. They are dropped from both sides of mask comparison
	// because the user never authored them — they exist purely because the
	// user set the sibling classic-API knob (e.g. self_service.security
	// authorization_password materialises as a com.apple.profileRemovalPassword
	// PayloadContent entry). Dropping them means the masked plan and the
	// masked server response stay length-aligned through Create/Update.
	//
	// Trade-off: a user who *manually* authors a payload entry of one of these
	// types (rare but possible) loses drift detection on it. The
	// MaskPayload-design note above explicitly rejects skip-lists for keys
	// users might author — this list intentionally accepts the carve-out
	// because every entry here is paired with a first-class classic-API field
	// the resource already exposes, and the in-payload form is redundant.
	serverInjectedPayloadTypes = map[string]struct{}{
		"com.apple.profileRemovalPassword": {}, // mirrors self_service.authorization_password
	}
	// mcxLikePayloadTypes enumerates PayloadType values whose inner
	// `.PayloadContent` child is opaque user-authored vendor preference data.
	// For these types, drift detection compares the inner subtree strictly
	// (keysets must match exactly) so admin-UI key add/remove inside the
	// vendor preference dict surfaces on the next plan. Apple's MCX format
	// (com.apple.ManagedClient.preferences) is the canonical case — Jamf Pro
	// UI exposes it as "Application & Custom Settings" / "Custom Settings".
	// Jamf is the transport for this subtree, not the editor: it never
	// injects metadata at the inner preference depth, so strict compare is
	// safe.
	mcxLikePayloadTypes = map[string]struct{}{
		"com.apple.ManagedClient.preferences": {},
	}
)

// MaskPayload returns a deep-cloned representation of the input plist with
// every server-controlled key dropped and every string value trimmed and
// line-ending-normalised (see normalizeLineEndings). The
// result is suitable for equality comparison against another masked payload
// — equality implies the two payloads are semantically the same modulo
// Jamf's well-known server-side normalisations.
//
// See STYLE_GUIDE.md §Configuration profile payload diff suppression for
// the diff-class catalogue this mask is designed to neutralise.
func MaskPayload(raw []byte) (map[string]any, error) {
	parsed, _, err := plisthelpers.ParsePlist(raw)
	if err != nil {
		return nil, err
	}
	return maskTopLevel(parsed), nil
}

func maskTopLevel(p map[string]any) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		if _, masked := maskedTopLevelKeys[k]; masked {
			continue
		}
		if k == "PayloadContent" {
			if arr, ok := v.([]any); ok {
				out[k] = maskPayloadContent(arr)
				continue
			}
		}
		trimmed := trimAny(v)
		if isEmpty(trimmed) {
			continue
		}
		out[k] = trimmed
	}
	return out
}

func maskPayloadContent(items []any) []any {
	out := make([]any, 0, len(items))
	for _, it := range items {
		dict, ok := it.(map[string]any)
		if !ok {
			out = append(out, trimAny(it))
			continue
		}
		if ptype, ok := dict["PayloadType"].(string); ok {
			if _, injected := serverInjectedPayloadTypes[ptype]; injected {
				continue
			}
		}
		masked := make(map[string]any, len(dict))
		for k, v := range dict {
			if _, drop := maskedPayloadContentKeys[k]; drop {
				continue
			}
			trimmed := trimAny(v)
			if isEmpty(trimmed) {
				continue
			}
			masked[k] = trimmed
		}
		out = append(out, masked)
	}
	return canonicalisePayloadContentOrder(out)
}

// canonicalisePayloadContentOrder stable-sorts a top-level PayloadContent array
// by PayloadType so two payloads that differ only in entry order compare equal
// under the positional array walk in LenientEqualPlist and structuralEqual.
//
// Jamf Pro does not store the array as submitted: it stably partitions the
// entries into the ones it stores verbatim followed by the ones it re-renders
// (see the storage-category table in importgate.go), keeping relative order
// within each block. Wire-probed 2026-08-11 against Jamf Pro 11.30.x over 18
// profile shapes: an authored [MCX, certificate] comes back as
// [certificate, MCX], while [loginwindow, certificate] — both verbatim slots —
// comes back untouched. A positional compare therefore pairs unrelated entries
// and reports cascading false drift on every key of both, which surfaced as a
// bogus "Jamf Pro cannot store this payload faithfully" error and a create-time
// rollback for a payload the server had stored perfectly.
//
// Sorting rather than special-casing the compare is safe because entry order in
// PayloadContent carries no meaning: Apple treats each entry as an independent
// payload, so an order-insensitive comparison is the correct one, not a
// tolerance. The sort is *stable* and keyed only on PayloadType, so two entries
// of the same type keep their authored order relative to each other and a real
// change between same-type siblings still surfaces as drift. That is exactly the
// granularity the wire law needs — same PayloadType implies the same partition,
// so Jamf Pro cannot reorder same-type siblings.
//
// PayloadIdentifier would be the natural pairing key but it is in
// maskedPayloadContentKeys (the server may assign it) and is already gone from
// the masked tree by the time this runs.
func canonicalisePayloadContentOrder(entries []any) []any {
	slices.SortStableFunc(entries, func(a, b any) int {
		return strings.Compare(payloadTypeOf(a), payloadTypeOf(b))
	})
	return entries
}

// payloadTypeOf returns a PayloadContent entry's PayloadType, or "" when the
// entry is not a dict or carries no type.
func payloadTypeOf(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	pt, _ := m["PayloadType"].(string)
	return pt
}

// isEmpty reports whether a plist value is equivalent to "user did not
// author anything" — empty string, nil, or empty collection.
func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

// normalizeBase64InString collapses line-wrap whitespace from string values
// that decode cleanly as base64. Apple-spec mobileconfig binary blobs live
// inside `<data>` tags (which howett.net/plist decodes to []byte and carry
// no whitespace), but some vendor payloads embed base64 inside `<string>`
// values — and Jamf Pro can line-wrap long base64 strings on the way back.
// Whitespace-only differences inside such strings would otherwise produce
// spurious diffs.
//
// Heuristic deliberately conservative — only fires when:
//   - The string contains an explicit newline (`\n` or `\r`). Plain spaces
//     do not trigger; natural-language values like "Allow 1Password
//     Launch Item" pass through untouched even when they happen to be
//     all-alphanumeric.
//   - After all-whitespace strip, the result is at least 32 characters and
//     a multiple of 4. Real base64 cert/blob content is nearly always far
//     longer; multi-line natural-language descriptions are vanishingly
//     unlikely to satisfy both length floor and length-mod-4.
//   - The cleaned form decodes successfully under standard base64.
//
// Anything that fails any gate is returned unchanged.
func normalizeBase64InString(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	clean := strings.Join(strings.Fields(s), "")
	if len(clean) < 32 || len(clean)%4 != 0 {
		return s
	}
	if _, err := base64.StdEncoding.DecodeString(clean); err == nil {
		return clean
	}
	return s
}

// normalizeLineEndings rewrites `\r\n` and lone `\r` to `\n` so a carriage
// return compares equal to a line feed inside string values. A CR is authored
// as a `&#13;` reference (the only whitespace character Jamf Pro keeps) but
// always reads back as LF: MCX and mobile payload fragments normalise it on
// store, and a verbatim-stored CR returns as a raw CR byte our own parse
// normalises. Without this every `&#13;` fails read-back verification.
// U+2028/U+2029/U+0085 are left alone — they round-trip byte-exact.
// See STYLE_GUIDE §Configuration profile payload diff suppression.
func normalizeLineEndings(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	return crlfReplacer.Replace(s)
}

// crlfReplacer matches `\r\n` before lone `\r` — strings.Replacer tries its
// pairs in argument order at each position, so CRLF never becomes two LFs.
var crlfReplacer = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// trimAny recursively trims whitespace from every string value in the plist
// tree. For ordinary text values this is `strings.TrimSpace` over
// line-ending-normalised text (see normalizeLineEndings). For string
// values that decode as base64, internal whitespace is also collapsed (see
// normalizeBase64InString) so server-side line wrapping does not produce
// spurious diffs.
func trimAny(v any) any {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(normalizeBase64InString(normalizeLineEndings(t)))
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = trimAny(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = trimAny(val)
		}
		return out
	default:
		return v
	}
}

// PayloadsSemanticallyEqual returns true when the two raw mobileconfig
// payloads are equal after skipping server-controlled fields and treating
// asymmetric dict keys as a no-op (intersection compare).
//
// The intersection compare is the formal expression of the user's
// "ignore overridden fields, no mapping tables" directive: when Jamf Pro
// adds or drops a key the user didn't author, both sides differ in their
// key-sets — but a real semantic change is in the *value* of a key that
// is on both sides. The compare therefore walks the union of keys at every
// dict, but only fails when a key present on both sides has differing
// values.
//
// Trade-off: a user who edits only a key Jamf Pro itself derives (e.g.
// tunes `PayloadOrganization` from empty to a value, then back) won't see
// a diff — the server will substitute its own value, so flagging that as
// drift would produce a persistent diff loop.
func PayloadsSemanticallyEqual(a, b []byte) (bool, error) {
	ma, err := MaskPayload(a)
	if err != nil {
		return false, fmt.Errorf("masking left side: %w", err)
	}
	mb, err := MaskPayload(b)
	if err != nil {
		return false, fmt.Errorf("masking right side: %w", err)
	}
	return LenientEqualPlist(ma, mb), nil
}

// LenientEqualPlist compares two parsed-and-masked plist trees with
// intersection semantics: dict keys present on only one side are ignored;
// shared keys must compare equal. Arrays compare positionally — the top-level
// PayloadContent array, the one array Jamf Pro reorders on store, is put into a
// canonical order by MaskPayload before it gets here (see
// canonicalisePayloadContentOrder). Scalars compare via
// plisthelpers.NumericEqual for ints (howett.net/plist returns int64 or
// uint64 depending on sign).
//
// Exception: PayloadContent[i] entries whose PayloadType is in
// mcxLikePayloadTypes (e.g. com.apple.ManagedClient.preferences — Jamf
// Pro's "Application & Custom Settings") get their inner `.PayloadContent`
// child strict-compared via plisthelpers.Equal. That subtree is opaque
// user-authored vendor preference data; admin-UI key add/remove inside it
// is real drift and must surface on the next plan. Remaining keys at the
// MCX entry depth still use intersection so per-payload metadata defaults
// Jamf injects (e.g. PayloadEnabled) keep tolerating one-sided presence.
//
// Second exception: the Icon of a com.apple.webClip.managed entry is compared
// through iconsEquivalent rather than byte-for-byte, because Jamf Pro always
// re-renders it and no client can reproduce the result — see the wire law in
// webclipicon.go. This is the only data blob so treated; certificates, fonts
// and VPN secrets are stored verbatim and still compare exactly.
//
// Intersection compare elsewhere remains the documented trade-off: it
// keeps the corpus quiet on Jamf's many conditional-default injections
// (top-level `PayloadRemovalDisallowed`, per-payload `PayloadEnabled`,
// PPPC service strips, etc.) without having to enumerate every one.
func LenientEqualPlist(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return false
		}
		payloadType, _ := av["PayloadType"].(string)
		if payloadType != "" {
			if _, isMCX := mcxLikePayloadTypes[payloadType]; isMCX {
				ai, aHas := av["PayloadContent"]
				bi, bHas := bv["PayloadContent"]
				if aHas != bHas {
					return false
				}
				if aHas && !plisthelpers.Equal(ai, bi) {
					return false
				}
				for k, va := range av {
					if k == "PayloadContent" {
						continue
					}
					if vb, exists := bv[k]; exists {
						if !LenientEqualPlist(va, vb) {
							return false
						}
					}
				}
				return true
			}
		}
		for k, va := range av {
			vb, exists := bv[k]
			if !exists {
				continue
			}
			if isWebClipIcon(payloadType, k) {
				authored, stored, ok := iconBlobs(va, vb)
				if ok {
					if !iconsEquivalent(authored, stored) {
						return false
					}
					continue
				}
			}
			if !LenientEqualPlist(va, vb) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !LenientEqualPlist(av[i], bv[i]) {
				return false
			}
		}
		return true
	case []uint8:
		bv, ok := b.([]uint8)
		if !ok {
			return false
		}
		return bytes.Equal(av, bv)
	case uint64:
		return plisthelpers.NumericEqual(int64(av), b)
	case int64:
		return plisthelpers.NumericEqual(av, b)
	case int:
		return plisthelpers.NumericEqual(int64(av), b)
	default:
		return a == b
	}
}

// InjectTopLevelIdentifierValues overwrites the top-level PayloadUUID and
// PayloadIdentifier of newPayload with the supplied strings. The values
// should be the server-canonical identifiers sourced from state (e.g.
// state.General.UUID). Without this step, every PUT produces a new top-level
// UUID server-side and devices treat the update as a fresh install.
//
// If both uuid and identifier are empty (Create path), newPayload is
// returned unchanged.
func InjectTopLevelIdentifierValues(newPayload []byte, uuid, identifier string) ([]byte, error) {
	if uuid == "" && identifier == "" {
		return newPayload, nil
	}
	next, format, err := plisthelpers.ParsePlist(newPayload)
	if err != nil {
		return nil, fmt.Errorf("parsing new payload for identifier injection: %w", err)
	}
	if uuid != "" {
		next["PayloadUUID"] = uuid
	}
	if identifier != "" {
		next["PayloadIdentifier"] = identifier
	}
	buf := &bytes.Buffer{}
	enc := plist.NewEncoderForFormat(buf, format)
	enc.Indent("\t")
	if err := enc.Encode(next); err != nil {
		return nil, fmt.Errorf("re-serialising payload after identifier injection: %w", err)
	}
	return buf.Bytes(), nil
}

// StripTopLevelIdentifiers removes the top-level PayloadUUID and
// PayloadIdentifier from newPayload so Jamf Pro assigns fresh ones.
//
// An authored PayloadUUID is sent to Jamf Pro verbatim on create, and Jamf Pro
// requires it to be unique across profiles: two profiles built from one
// template (or one exported from Jamf Pro) answer the second create with
// `409 Duplicate payload uuid` (wire-probed 2026-10-27 against
// proclassic/mobiledeviceconfigurationprofiles, POST). Only the top-level UUID
// collides; a PayloadUUID repeated inside PayloadContent is accepted, and a
// payload that is otherwise identical is refused only because its top-level
// UUID repeats. When both are omitted Jamf Pro sets the two to the same fresh
// UUID, which it echoes as general.uuid.
//
// Nested payload identifiers and UUIDs are deliberately left alone: Jamf Pro
// keeps whatever it is sent, mints new ones on every write when they are
// omitted, and does not rewrite references between payloads (such as a Wi-Fi
// payload's PayloadCertificateAnchorUUID), so they belong to the author.
func StripTopLevelIdentifiers(newPayload []byte) ([]byte, error) {
	next, format, err := plisthelpers.ParsePlist(newPayload)
	if err != nil {
		return nil, fmt.Errorf("parsing new payload for identifier removal: %w", err)
	}
	_, hasUUID := next["PayloadUUID"]
	_, hasIdentifier := next["PayloadIdentifier"]
	if !hasUUID && !hasIdentifier {
		return newPayload, nil
	}
	delete(next, "PayloadUUID")
	delete(next, "PayloadIdentifier")
	buf := &bytes.Buffer{}
	enc := plist.NewEncoderForFormat(buf, format)
	enc.Indent("\t")
	if err := enc.Encode(next); err != nil {
		return nil, fmt.Errorf("re-serialising payload after identifier removal: %w", err)
	}
	return buf.Bytes(), nil
}

// PrepareCreateWirePayload produces the payload bytes sent on create:
// StripTopLevelIdentifiers followed by the same structural-whitespace
// compaction PrepareWirePayload applies, and for the same reason.
func PrepareCreateWirePayload(newPayload []byte) ([]byte, error) {
	prepared, err := StripTopLevelIdentifiers(newPayload)
	if err != nil {
		return nil, err
	}
	if compacted, cErr := plisthelpers.CompactStructuralWhitespace(prepared); cErr == nil {
		prepared = compacted
	}
	return prepared, nil
}

// PrepareWirePayload produces the payload bytes a configuration-profile
// resource actually sends to the Classic API: server-canonical identifier
// injection (InjectTopLevelIdentifierValues) followed by structural-whitespace
// compaction (plisthelpers.CompactStructuralWhitespace).
//
// Compaction exists because the Classic API's server-side plist parser
// materialises whitespace text nodes between sibling <array> tags as phantom
// empty <array/> entries in the stored plist (wire-probed against
// JSSResource/mobiledeviceconfigurationprofiles, 2026-06-10). It must run
// after identifier injection — that step re-serialises the plist
// pretty-printed, reintroducing the whitespace.
//
// Compaction is best-effort: when the payload is not well-formed XML the
// uncompacted bytes are returned so the server reports the malformation with
// its own error.
func PrepareWirePayload(newPayload []byte, uuid, identifier string) ([]byte, error) {
	prepared, err := InjectTopLevelIdentifierValues(newPayload, uuid, identifier)
	if err != nil {
		return nil, err
	}
	if compacted, cErr := plisthelpers.CompactStructuralWhitespace(prepared); cErr == nil {
		prepared = compacted
	}
	return prepared, nil
}

// InjectTopLevelIdentifiers is the existingPayload-based wrapper used by
// unit tests. CRUD callers use InjectTopLevelIdentifierValues directly so
// they can pass server-canonical identifiers sourced from state.General.UUID.
func InjectTopLevelIdentifiers(newPayload, existingPayload []byte) ([]byte, error) {
	if len(existingPayload) == 0 {
		return newPayload, nil
	}
	exist, _, err := plisthelpers.ParsePlist(existingPayload)
	if err != nil {
		return newPayload, nil //nolint:nilerr // best-effort
	}
	uuid, _ := exist["PayloadUUID"].(string)
	identifier, _ := exist["PayloadIdentifier"].(string)
	return InjectTopLevelIdentifierValues(newPayload, uuid, identifier)
}

// ExtractServerPayloadFromGeneral pulls the mobileconfig plist text out of a
// classic API GET response. The server returns the payload inside
// <general><payloads>...</payloads></general> as XML-entity-encoded text (not
// CDATA). This helper exists for tests that work directly with captured wire XML.
func ExtractServerPayloadFromGeneral(wireXML []byte) ([]byte, error) {
	const (
		openTag  = "<payloads>"
		closeTag = "</payloads>"
	)
	si := bytes.Index(wireXML, []byte(openTag))
	ei := bytes.Index(wireXML, []byte(closeTag))
	if si < 0 || ei < 0 || si >= ei {
		return nil, fmt.Errorf("no <payloads> block found")
	}
	inner := wireXML[si+len(openTag) : ei]
	inner = bytes.TrimSpace(inner)
	if bytes.HasPrefix(inner, []byte("<![CDATA[")) && bytes.HasSuffix(inner, []byte("]]>")) {
		return inner[len("<![CDATA[") : len(inner)-len("]]>")], nil
	}
	return []byte(html.UnescapeString(string(inner))), nil
}

// PayloadFidelitySummary is the diagnostic summary both profile resources use
// for a read-back verification failure, on create and on update. The detail is
// built per failure by PayloadFidelityErrorDetail (see fidelity.go), which
// names the diverging values and the remedy for each.
const PayloadFidelitySummary = "Jamf Pro cannot store this payload faithfully"
