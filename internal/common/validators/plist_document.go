// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// plistWrapperExample is the wrapper the remediation text tells users to add.
// `version="1.0"` is Apple's canonical value; Jamf Pro stores it as `1` and the
// payload mask normalises the difference, so either authored form is fine.
const plistWrapperExample = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  …your existing content…
</plist>`

// PlistDocument checks that a string attribute holds a complete plist document
// — one whose root element is `<plist>` — rather than a bare fragment such as a
// lone `<dict>…</dict>`.
//
// Why this exists: Jamf Pro's configuration-profile endpoints reject a fragment
// with HTTP 409 "Unable to update the database", which tells the user nothing
// about the actual mistake. Wire-probed against a live tenant 2026-08-24
// (issue #326): the same payload is accepted the moment a `<plist>` root is
// added, and the root element alone is sufficient — the XML declaration and the
// DOCTYPE are both optional. So the root element is the gate, and neither of the
// other two is checked.
//
// The validator only errors when it positively identifies a wrong root element
// (or no element at all). Anything it cannot parse is deferred to the server:
// false-rejecting a payload Jamf Pro would have accepted is worse than passing
// an unparseable one through to the existing error path. Null/unknown values are
// deferred per STYLE_GUIDE §Config-time validators.
//
// Why plan time and not create time: a fragment used to fail on create yet
// succeed on update, which reads as a create/update difference in Jamf Pro but
// is not one — a raw PUT of the same fragment to an existing profile 409s
// identically (wire-probed 2026-08-24). The asymmetry was ours.
// payloadhelpers.InjectTopLevelIdentifierValues returns early on create (no
// identifiers to inject yet) and sends the authored bytes untouched — still the
// case for the macOS resource; the mobile device resource now strips the
// top-level identifiers instead, which parses too — but on update it parses
// the payload — plisthelpers.ParsePlist accepts a bare dict fragment — and
// re-encodes it, and the encoder always emits a whole document. So update
// silently repaired the payload and create could not. Gating in the
// schema closes that gap from both directions: the fragment never reaches
// either path, so neither can disagree about it again.
func PlistDocument() validator.String {
	return plistDocumentValidator{}
}

type plistDocumentValidator struct{}

// Description returns a plain-text description of the validator.
func (plistDocumentValidator) Description(_ context.Context) string {
	return "value must be a complete plist document whose root element is <plist>."
}

// MarkdownDescription returns the markdown description of the validator.
func (v plistDocumentValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (plistDocumentValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()
	if strings.TrimSpace(raw) == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Empty payload",
			"The payload is empty. Supply the plist document describing the settings this profile delivers.",
		)
		return
	}
	// A binary plist carries no XML root element to inspect. Defer rather than
	// false-reject.
	if strings.HasPrefix(raw, "bplist00") {
		return
	}

	root, ok := xmlRootElement(raw)
	if !ok {
		return // Unparseable — leave it to Jamf Pro to reject.
	}
	if root == "plist" {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Payload is not a complete plist document",
		fmt.Sprintf(
			"The payload starts with <%s>, so it is a fragment rather than a whole plist document. "+
				"Jamf Pro refuses a fragment with \"Unable to update the database\" and reports nothing more specific.\n\n"+
				"Wrap the payload:\n\n%s",
			root, plistWrapperExample,
		),
	)
}

// xmlRootElement returns the local name of the first element in raw. ok is false
// when raw holds no element or cannot be tokenised, in which case the caller must
// not report an error — see PlistDocument's contract.
func xmlRootElement(raw string) (name string, ok bool) {
	dec := xml.NewDecoder(strings.NewReader(raw))
	// Accept any declared encoding. Only element names are read, and those are
	// ASCII in every plist, so decoding the bytes as-is is sufficient. Without
	// this, `encoding="ISO-8859-1"` makes Token fail and the whole payload would
	// be deferred for a reason unrelated to its structure.
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	// Jamf payloads reference Apple's DTD by URL; entity resolution is not
	// needed to find the root element and must never trigger a network fetch.
	dec.Strict = false
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", false
		}
		if start, isStart := tok.(xml.StartElement); isStart {
			return start.Name.Local, true
		}
	}
}
