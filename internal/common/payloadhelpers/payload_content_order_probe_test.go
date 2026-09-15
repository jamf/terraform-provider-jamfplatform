// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build payload_probe

// Wire-probe harness for the PayloadContent ordering law behind
// canonicalisePayloadContentOrder, and for the drift detection that ordering
// must not weaken. Not part of any normal build — it creates, updates and
// deletes real profiles on a live tenant.
//
//	JAMFPLATFORM_BASE_URL=... JAMFPLATFORM_CLIENT_ID=... \
//	JAMFPLATFORM_CLIENT_SECRET=... JAMFPLATFORM_TENANT_ID=... \
//	go test -tags payload_probe ./internal/common/payloadhelpers/ -run TestProbePayloadContent -v
//
// TestProbePayloadContentOrder asserts the law itself: Jamf Pro stably partitions
// the top-level PayloadContent array into the entries it stores verbatim followed
// by the entries it re-renders, preserving relative order within each block. The
// expected order is derived from faithfulPayloadTypes, so if the two ever
// disagree this probe says which entry moved where.
//
// TestProbePayloadContentOrderDoesNotHideDrift is the other half: it applies the
// admin-UI edits an operator can make to a stored profile and asserts each one is
// still caught, by the plan-time gate (PayloadsSemanticallyEqual) or the
// Read-side drift detector (PayloadsStructurallyEqual) or both. A canonical
// ordering is only safe if the sole thing it hides is a pure permutation.
package payloadhelpers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/plisthelpers"
)

// orderRoundTrip creates a profile, reads it back, and registers its deletion.
// Unlike roundTripProbe in storage_category_probe_test.go it hands the ID back,
// because the drift probe below has to update the profile before it is removed.
func orderRoundTrip(t *testing.T, ctx context.Context, c *proclassic.Client, name string, authored []byte) (stored []byte, id string) {
	t.Helper()
	prepared, err := PrepareWirePayload(authored, "", "")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	pl := proclassic.PayloadsXMLText(prepared)
	level := "System"
	created, err := c.CreateOSXConfigurationProfileByID(ctx, "0",
		&proclassic.OsXConfigurationProfile{General: &proclassic.OsXConfigurationProfileGeneral{
			Name: &name, Payloads: &pl, Level: &level}})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	id = probeID(created.ID, func() *int {
		if created.General != nil {
			return created.General.ID
		}
		return nil
	}())
	if id == "" {
		t.Fatal("create returned no id")
	}
	t.Cleanup(func() {
		if err := c.DeleteOSXConfigurationProfileByID(ctx, id); err != nil {
			t.Errorf("probe profile %s left behind: %v", id, err)
		}
	})
	got, err := c.GetOSXConfigurationProfileByID(ctx, id)
	if err != nil || got == nil || got.General == nil || got.General.Payloads == nil {
		t.Fatalf("read back %s: %v", id, err)
	}
	return []byte(string(*got.General.Payloads)), id
}

// orderEntry is one PayloadContent entry: a payload type plus a marker that
// makes two entries of the same type distinguishable in the probe output.
type orderEntry struct {
	ptype string
	mark  string
}

func (e orderEntry) label() string {
	return e.ptype[strings.LastIndexByte(e.ptype, '.')+1:] + "/" + e.mark
}

// xml renders a body the server actually keeps. A payload with no content keys
// is silently discarded on store, which would make every assertion below vacuous.
func (e orderEntry) xml() string {
	switch e.ptype {
	case "com.apple.ManagedClient.preferences":
		return fmt.Sprintf(`<dict>
<key>PayloadType</key><string>%s</string><key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>com.zz.order.%s</string>
<key>PayloadUUID</key><string>ZZORDER-%s</string>
<key>PayloadContent</key><dict><key>com.zz.probe.%s</key><dict><key>Forced</key><array>
<dict><key>mcx_preference_settings</key><dict><key>ZZmark</key><string>%s</string></dict></dict>
</array></dict></dict></dict>`, e.ptype, e.mark, e.mark, e.mark, e.mark)
	case "com.apple.security.root":
		return fmt.Sprintf(`<dict>
<key>PayloadType</key><string>%s</string><key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>com.zz.order.%s</string>
<key>PayloadUUID</key><string>ZZORDER-%s</string>
<key>PayloadCertificateFileName</key><string>%s.cer</string>
<key>PayloadContent</key><data>
MIIBCgKCAQEAxGF3lQ0m6zvbNQ0dQKe9nJ6l0J1yQ4nFq0dYh2s5cWJ9Yz8mQ2pL
</data></dict>`, e.ptype, e.mark, e.mark, e.mark)
	case "com.apple.notificationsettings":
		return fmt.Sprintf(`<dict>
<key>PayloadType</key><string>%s</string><key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>com.zz.order.%s</string>
<key>PayloadUUID</key><string>ZZORDER-%s</string>
<key>NotificationSettings</key><array><dict>
<key>AlertType</key><integer>1</integer>
<key>BundleIdentifier</key><string>com.zz.probe.%s</string>
<key>NotificationsEnabled</key><true/>
</dict></array></dict>`, e.ptype, e.mark, e.mark, e.mark)
	case "com.apple.loginwindow":
		return fmt.Sprintf(`<dict>
<key>PayloadType</key><string>%s</string><key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>com.zz.order.%s</string>
<key>PayloadUUID</key><string>ZZORDER-%s</string>
<key>LoginwindowText</key><string>%s</string></dict>`, e.ptype, e.mark, e.mark, e.mark)
	default:
		panic("orderEntry: no probe body for " + e.ptype)
	}
}

func orderProfile(name string, entries ...orderEntry) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string><key>PayloadVersion</key><integer>1</integer>
<key>PayloadScope</key><string>System</string>
<key>PayloadDisplayName</key><string>%s</string>
<key>PayloadIdentifier</key><string>com.zz.order.top</string>
<key>PayloadUUID</key><string>ZZORDER-TOP</string>
<key>PayloadContent</key><array>
`, name)
	for _, e := range entries {
		b.WriteString(e.xml())
		b.WriteString("\n")
	}
	b.WriteString("</array>\n</dict></plist>")
	return []byte(b.String())
}

// expectedStoredOrder is the wire law as code: a stable partition putting the
// verbatim-stored entries first, then the re-rendered ones.
func expectedStoredOrder(platform ProfilePlatform, entries []orderEntry) []string {
	var verbatim, rerendered []string
	for _, e := range entries {
		if isFaithfulPayloadType(platform, e.ptype) {
			rerendered = append(rerendered, e.label())
		} else {
			verbatim = append(verbatim, e.label())
		}
	}
	return append(verbatim, rerendered...)
}

// storedOrder labels each stored entry the same way, pairing it back to the
// authored entry by the marker the body carries.
func storedOrder(t *testing.T, raw []byte) []string {
	t.Helper()
	tree, _, err := plisthelpers.ParsePlist(raw)
	if err != nil {
		t.Fatalf("parsing stored payload: %v", err)
	}
	arr, _ := tree["PayloadContent"].([]any)
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			out = append(out, "?/non-dict")
			continue
		}
		pt := payloadTypeOf(item)
		short := pt[strings.LastIndexByte(pt, '.')+1:]
		out = append(out, short+"/"+storedMarker(m))
	}
	return out
}

// storedMarker digs the probe marker back out of whichever slot the payload type
// keeps it in.
func storedMarker(m map[string]any) string {
	if s, ok := m["PayloadCertificateFileName"].(string); ok {
		return strings.TrimSuffix(s, ".cer")
	}
	if s, ok := m["LoginwindowText"].(string); ok {
		return s
	}
	if arr, ok := m["NotificationSettings"].([]any); ok && len(arr) > 0 {
		if d, ok := arr[0].(map[string]any); ok {
			if s, ok := d["BundleIdentifier"].(string); ok {
				return strings.TrimPrefix(s, "com.zz.probe.")
			}
		}
	}
	if inner, ok := m["PayloadContent"].(map[string]any); ok {
		for k := range inner {
			return strings.TrimPrefix(k, "com.zz.probe.")
		}
	}
	return "?"
}

func TestProbePayloadContentOrder(t *testing.T) {
	pc := probeClient(t)
	ctx := context.Background()

	mcxA := orderEntry{"com.apple.ManagedClient.preferences", "mcxa"}
	mcxB := orderEntry{"com.apple.ManagedClient.preferences", "mcxb"}
	mcxC := orderEntry{"com.apple.ManagedClient.preferences", "mcxc"}
	certA := orderEntry{"com.apple.security.root", "certa"}
	certB := orderEntry{"com.apple.security.root", "certb"}
	notif := orderEntry{"com.apple.notificationsettings", "notif"}
	login := orderEntry{"com.apple.loginwindow", "login"}

	shapes := []struct {
		name    string
		entries []orderEntry
	}{
		{"mcx_then_cert", []orderEntry{mcxA, certA}},
		{"cert_then_mcx_already_canonical", []orderEntry{certA, mcxA}},
		{"notif_then_cert", []orderEntry{notif, certA}},
		{"both_verbatim_untouched", []orderEntry{login, certA}},
		{"both_rerendered_untouched", []orderEntry{notif, mcxA}},
		{"mcx_login_cert", []orderEntry{mcxA, login, certA}},
		{"same_type_siblings_untouched", []orderEntry{mcxA, mcxB, mcxC}},
		{"interleaved_two_of_each", []orderEntry{mcxA, certA, mcxB, certB, mcxC}},
		{"two_verbatim_two_rerendered", []orderEntry{certA, notif, login, mcxA}},
	}

	suffix := strconv.FormatInt(time.Now().Unix(), 10)
	for _, s := range shapes {
		t.Run(s.name, func(t *testing.T) {
			name := "zz-order-probe-" + s.name + "-" + suffix
			authored := orderProfile(name, s.entries...)
			stored, _ := orderRoundTrip(t, ctx, pc, name, authored)

			want := expectedStoredOrder(PlatformMacOS, s.entries)
			got := storedOrder(t, stored)
			var authoredLabels []string
			for _, e := range s.entries {
				authoredLabels = append(authoredLabels, e.label())
			}
			t.Logf("authored %v", authoredLabels)
			t.Logf("stored   %v", got)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("stored order does not match the verbatim-then-re-rendered partition\n  want %v\n  got  %v\n"+
					"faithfulPayloadTypes and the server disagree — re-probe the storage categories before trusting the compare",
					want, got)
			}

			// Whatever the server did with the order, the compare must be quiet:
			// nothing about these payloads changed.
			eq, err := PayloadsSemanticallyEqual(authored, stored)
			if err != nil {
				t.Fatalf("compare: %v", err)
			}
			if !eq {
				t.Errorf("reorder alone must not read as drift")
			}
			if findings, ok := diffPayloadStrings(authored, stored); ok && len(findings) > 0 {
				for _, f := range findings {
					t.Logf("unexpected finding: %s (present=%v)", f.path, f.present)
				}
				t.Errorf("reorder alone must not produce fidelity findings, got %d", len(findings))
			}
		})
	}
}

// TestProbePayloadContentOrderDoesNotHideDrift applies each admin-UI edit an
// operator can make to a stored profile and asserts it is still detected. Pure
// permutation is the one case that must NOT be reported: entry order carries no
// meaning to Apple, and the server produces it unprompted on every write.
func TestProbePayloadContentOrderDoesNotHideDrift(t *testing.T) {
	pc := probeClient(t)
	ctx := context.Background()

	mcxA := orderEntry{"com.apple.ManagedClient.preferences", "mcxa"}
	mcxB := orderEntry{"com.apple.ManagedClient.preferences", "mcxb"}
	certA := orderEntry{"com.apple.security.root", "certa"}
	notif := orderEntry{"com.apple.notificationsettings", "notif"}

	base := []orderEntry{mcxA, certA}

	cases := []struct {
		name       string
		edited     []orderEntry
		mutate     func(string) string
		wantDetect bool
	}{
		{name: "value changed inside a re-rendered payload", edited: base,
			mutate: func(s string) string {
				return strings.Replace(s, "<string>mcxa</string>", "<string>EDITED</string>", 1)
			},
			wantDetect: true},
		{name: "key added inside a re-rendered payload", edited: base,
			mutate: func(s string) string {
				return strings.Replace(s, "<key>ZZmark</key>", "<key>ZZextra</key><string>added</string><key>ZZmark</key>", 1)
			},
			wantDetect: true},
		{name: "key removed inside a re-rendered payload", edited: base,
			mutate: func(s string) string {
				return strings.Replace(s, "<key>ZZmark</key><string>mcxa</string>", "<key>ZZother</key><string>mcxa</string>", 1)
			},
			wantDetect: true},
		{name: "value changed inside a verbatim payload", edited: base,
			mutate:     func(s string) string { return strings.Replace(s, "certa.cer", "swapped.cer", 1) },
			wantDetect: true},
		{name: "payload added", edited: []orderEntry{mcxA, certA, notif}, wantDetect: true},
		{name: "payload removed", edited: []orderEntry{mcxA}, wantDetect: true},
		{name: "payload swapped, count unchanged", edited: []orderEntry{mcxA, notif}, wantDetect: true},
		{name: "second of two same-type siblings edited",
			edited: []orderEntry{mcxA, mcxB},
			mutate: func(s string) string {
				return strings.Replace(s, "<string>mcxb</string>", "<string>EDITED</string>", 1)
			},
			wantDetect: true},
		{name: "pure permutation, nothing else changed", edited: []orderEntry{certA, mcxA}, wantDetect: false},
	}

	suffix := strconv.FormatInt(time.Now().Unix(), 10)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := "zz-drift-probe-" + strconv.Itoa(len(tc.name)) + "-" + suffix
			authoredEntries := base
			if tc.mutate != nil {
				authoredEntries = tc.edited
			}
			authored := orderProfile(name, authoredEntries...)

			// Apply: the profile Terraform created, and the server form it saw back.
			atApply, id := orderRoundTrip(t, ctx, pc, name, authored)

			// Admin edit, via the same API a UI save goes through.
			edited := string(orderProfile(name, tc.edited...))
			if tc.mutate != nil {
				edited = tc.mutate(edited)
			}
			if edited == string(authored) {
				t.Fatal("edit was a no-op — the probe body moved and this case covers nothing")
			}
			wire, err := PrepareWirePayload([]byte(edited), "", "")
			if err != nil {
				t.Fatal(err)
			}
			pxt := proclassic.PayloadsXMLText(wire)
			if err := pc.UpdateOSXConfigurationProfileByID(ctx, id, &proclassic.OsXConfigurationProfile{
				General: &proclassic.OsXConfigurationProfileGeneral{Payloads: &pxt},
			}); err != nil {
				t.Fatalf("update: %v", err)
			}
			got, err := pc.GetOSXConfigurationProfileByID(ctx, id)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			serverNow := []byte(string(*got.General.Payloads))

			planEq, err := PayloadsSemanticallyEqual(authored, serverNow)
			if err != nil {
				t.Fatalf("plan-gate compare: %v", err)
			}
			readEq, err := PayloadsStructurallyEqual(atApply, serverNow)
			if err != nil {
				t.Fatalf("read-drift compare: %v", err)
			}
			detected := !planEq || !readEq
			t.Logf("detected=%-5v (plan gate differs=%-5v, read-side drift differs=%-5v)", detected, !planEq, !readEq)
			if detected != tc.wantDetect {
				t.Errorf("%s: detected=%v want=%v — stored order %v",
					tc.name, detected, tc.wantDetect, storedOrder(t, serverNow))
			}
		})
	}
}
