// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build payload_probe

// Wire-probe harness for the configuration-profile storage-category table in
// importgate.go. Not part of any normal build — it creates and deletes real
// profiles on a live tenant.
//
//	JAMFPLATFORM_BASE_URL=... JAMFPLATFORM_CLIENT_ID=... \
//	JAMFPLATFORM_CLIENT_SECRET=... JAMFPLATFORM_TENANT_ID=... \
//	go test -tags payload_probe ./internal/common/payloadhelpers/ -run TestProbeStorageCategories -v
//
// For each (platform, PayloadType) it creates a profile carrying "&", "<", ">",
// LF, TAB and a CR reference in a string value, reads the profile back, and
// classifies the type:
//
//	faithful → re-render: add it to faithfulPayloadTypes for that platform.
//	LOSSY    → verbatim:  leave it out (deny-by-default already covers it).
//	DROPPED / CREATE-REJECTED → inconclusive. The generic probe payload is not
//	  valid for that type, so the server refused it or discarded the probe keys.
//	  Re-probe with TestProbeStorageCategoriesFromRealPayload, which round-trips a
//	  real payload lifted off a tenant instead of a synthetic one — that is how
//	  com.apple.webcontent-filter was classified.
//
// Only add a table entry off the back of a probe result. Inferring that a type
// "looks like" a re-render type is how a false positive gets shipped, and a
// wrong faithful entry is the dangerous direction: it lets a profile through to
// be corrupted by its first update.
package payloadhelpers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/plisthelpers"
)

// probeKeys are the six values under test and the wire law each one checks.
var probeKeys = map[string]string{
	"ZZamp": "A&B",     // PI-827 extra entity layer
	"ZZlt":  "C<D",     // PI-827 extra entity layer
	"ZZgt":  "E>F",     // expected to survive everywhere
	"ZZlf":  "G\nH",    // deleted in verbatim slots
	"ZZtab": "I\tJ",    // deleted in verbatim slots
	"ZZcr":  "K&#13;L", // CR reference: expected to survive everywhere
}

// probeOrder fixes the reporting order, since map iteration is randomised.
var probeOrder = []string{"ZZamp", "ZZlt", "ZZgt", "ZZlf", "ZZtab", "ZZcr"}

// probeTypes lists the payload types to classify per platform. Extend it when a
// new type needs a verdict.
var probeTypes = map[ProfilePlatform][]string{
	PlatformMacOS: {
		"com.apple.ManagedClient.preferences", "com.apple.notificationsettings",
		"com.apple.systempolicy.control", "com.apple.TCC.configuration-profile-policy",
		"com.apple.loginwindow", "com.apple.extensiblesso", "com.apple.vpn.managed",
		"com.apple.wifi.managed", "com.apple.applicationaccess", "com.apple.servicemanagement",
		"com.apple.system-extension-policy", "com.apple.security.firewall", "com.apple.dock",
		"com.apple.screensaver", "com.apple.MCX", "com.apple.SubmitDiagInfo",
		"com.apple.mobiledevice.passwordpolicy", "com.apple.webcontent-filter",
	},
	PlatformMobileDevice: {
		"com.apple.ManagedClient.preferences", "com.apple.notificationsettings",
		"com.apple.webClip.managed", "com.apple.applicationaccess", "com.apple.wifi.managed",
		"com.apple.vpn.managed", "com.apple.webcontent-filter",
		"com.apple.shareddeviceconfiguration", "com.apple.mobiledevice.passwordpolicy",
	},
}

func probeClient(t *testing.T) *proclassic.Client {
	t.Helper()
	base, id, secret := os.Getenv("JAMFPLATFORM_BASE_URL"), os.Getenv("JAMFPLATFORM_CLIENT_ID"), os.Getenv("JAMFPLATFORM_CLIENT_SECRET")
	if base == "" || id == "" || secret == "" {
		t.Skip("probe needs JAMFPLATFORM_BASE_URL / _CLIENT_ID / _CLIENT_SECRET")
	}
	var opts []jamfplatform.Option
	switch eid, tid := os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID"), os.Getenv("JAMFPLATFORM_TENANT_ID"); {
	case eid != "":
		opts = append(opts, jamfplatform.WithEnvironmentID(eid))
	case tid != "":
		opts = append(opts, jamfplatform.WithTenantID(tid))
	}
	c := jamfplatform.NewClient(base, id, secret, opts...)
	if err := c.ValidateCredentials(context.Background()); err != nil {
		t.Fatalf("credentials: %v", err)
	}
	return proclassic.New(c)
}

// escapePlistSource escapes a probe value for plist source text.
func escapePlistSource(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// buildProbePlist wraps one payload type around the probe values. For MCX the
// values go in the inner vendor-preference dict, the subtree that carries
// user-authored data for that type; for everything else they are direct keys.
func buildProbePlist(ptype, uuidTop, uuidEntry string) string {
	var vals strings.Builder
	for _, k := range probeOrder {
		v := probeKeys[k]
		if k == "ZZcr" {
			// Already a character reference — escaping it would defeat the test.
			fmt.Fprintf(&vals, "<key>%s</key><string>%s</string>", k, v)
			continue
		}
		fmt.Fprintf(&vals, "<key>%s</key><string>%s</string>", k, escapePlistSource(v))
	}

	inner := vals.String()
	if ptype == "com.apple.ManagedClient.preferences" {
		inner = "<key>PayloadContent</key><dict><key>com.zz.probe</key><dict><key>Forced</key><array>" +
			"<dict><key>mcx_preference_settings</key><dict>" + inner + "</dict></dict></array></dict></dict>"
	}

	return `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" ` +
		`"http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict>` +
		`<key>PayloadContent</key><array><dict>` +
		`<key>PayloadType</key><string>` + ptype + `</string>` +
		`<key>PayloadVersion</key><integer>1</integer>` +
		`<key>PayloadIdentifier</key><string>` + uuidEntry + `</string>` +
		`<key>PayloadUUID</key><string>` + uuidEntry + `</string>` +
		inner +
		`</dict></array>` +
		`<key>PayloadType</key><string>Configuration</string>` +
		`<key>PayloadVersion</key><integer>1</integer>` +
		`<key>PayloadScope</key><string>System</string>` +
		`<key>PayloadIdentifier</key><string>` + uuidTop + `</string>` +
		`<key>PayloadUUID</key><string>` + uuidTop + `</string>` +
		`</dict></plist>`
}

// roundTripProbe creates a profile with the given payload, reads it back, and
// deletes it. Returns the stored payload, or ok=false when the server refused
// the create.
func roundTripProbe(t *testing.T, c *proclassic.Client, platform ProfilePlatform, name, plistText string) (stored []byte, ok bool) {
	t.Helper()
	ctx := context.Background()
	prepared, err := PrepareWirePayload([]byte(plistText), "", "")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	pl := proclassic.PayloadsXMLText(prepared)
	level := "System"

	if platform == PlatformMobileDevice {
		created, cErr := c.CreateMobileDeviceConfigurationProfileByID(ctx, "0",
			&proclassic.MobileDeviceConfigurationProfile{General: &proclassic.MobileDeviceConfigurationProfileGeneral{
				Name: &name, Payloads: &pl, Level: &level}})
		if cErr != nil {
			return nil, false
		}
		id := probeID(created.ID, func() *int {
			if created.General != nil {
				return created.General.ID
			}
			return nil
		}())
		defer func() { _ = c.DeleteMobileDeviceConfigurationProfileByID(ctx, id) }()
		got, gErr := c.GetMobileDeviceConfigurationProfileByID(ctx, id)
		if gErr != nil || got == nil || got.General == nil || got.General.Payloads == nil {
			t.Fatalf("read back %s: %v", id, gErr)
		}
		return []byte(string(*got.General.Payloads)), true
	}

	created, cErr := c.CreateOSXConfigurationProfileByID(ctx, "0",
		&proclassic.OsXConfigurationProfile{General: &proclassic.OsXConfigurationProfileGeneral{
			Name: &name, Payloads: &pl, Level: &level}})
	if cErr != nil {
		return nil, false
	}
	id := probeID(created.ID, func() *int {
		if created.General != nil {
			return created.General.ID
		}
		return nil
	}())
	defer func() { _ = c.DeleteOSXConfigurationProfileByID(ctx, id) }()
	got, gErr := c.GetOSXConfigurationProfileByID(ctx, id)
	if gErr != nil || got == nil || got.General == nil || got.General.Payloads == nil {
		t.Fatalf("read back %s: %v", id, gErr)
	}
	return []byte(string(*got.General.Payloads)), true
}

func probeID(root, general *int) string {
	if root != nil {
		return fmt.Sprintf("%d", *root)
	}
	if general != nil {
		return fmt.Sprintf("%d", *general)
	}
	return ""
}

// classifyProbeValue names the wire law that explains one authored/stored pair.
func classifyProbeValue(want, got string, present bool) string {
	switch {
	case !present:
		return "DROPPED"
	case got == want,
		strings.TrimSpace(got) == strings.TrimSpace(want),
		// CR normalises to LF on read for re-render types; normalizeLineEndings
		// already treats the two as equal, so this is not a divergence.
		normalizeLineEndings(got) == normalizeLineEndings(want):
		return "faithful"
	case applyVerbatimStorage(want) == got:
		return "LOSSY(verbatim)"
	default:
		return fmt.Sprintf("OTHER(want=%q got=%q)", want, got)
	}
}

// findLeafByKey returns the flattened path whose final segment is key.
func findLeafByKey(flat map[string]string, key string) (string, bool) {
	for p := range flat {
		if strings.HasSuffix(p, "."+key) {
			return p, true
		}
	}
	return "", false
}

func TestProbeStorageCategories(t *testing.T) {
	c := probeClient(t)

	for _, platform := range []ProfilePlatform{PlatformMacOS, PlatformMobileDevice} {
		for i, ptype := range probeTypes[platform] {
			uuidTop := fmt.Sprintf("PROBE0-%d-%04d-TOP", platform, i)
			uuidEntry := fmt.Sprintf("PROBE0-%d-%04d-ENT", platform, i)
			plistText := buildProbePlist(ptype, uuidTop, uuidEntry)

			stored, ok := roundTripProbe(t, c, platform,
				fmt.Sprintf("ZZ storage probe %d %s", platform, ptype), plistText)
			if !ok {
				t.Logf("%-6s %-46s CREATE-REJECTED (probe payload invalid for this type — use the real-payload probe)",
					platformName(platform), ptype)
				continue
			}

			authoredTree, _, aErr := plisthelpers.ParsePlist([]byte(plistText))
			storedTree, _, sErr := plisthelpers.ParsePlist(stored)
			if aErr != nil || sErr != nil {
				t.Fatalf("%s parse: authored=%v stored=%v", ptype, aErr, sErr)
			}
			authoredFlat, storedFlat := map[string]string{}, map[string]string{}
			flattenStringLeaves("", authoredTree, authoredFlat)
			flattenStringLeaves("", storedTree, storedFlat)

			verdicts := make([]string, 0, len(probeOrder))
			for _, k := range probeOrder {
				ap, found := findLeafByKey(authoredFlat, k)
				if !found {
					verdicts = append(verdicts, k+"=?authored-missing")
					continue
				}
				got, present := "", false
				if sp, ok := findLeafByKey(storedFlat, k); ok {
					got, present = storedFlat[sp], true
				}
				verdicts = append(verdicts, k+"="+classifyProbeValue(authoredFlat[ap], got, present))
			}
			t.Logf("%-6s %-46s %s", platformName(platform), ptype, strings.Join(verdicts, "  "))
		}
	}
}

// TestProbeStorageCategoriesFromRealPayload classifies a type whose synthetic
// probe is rejected, by round-tripping a real payload with hazard characters
// injected into one of its own string fields. Point ZZ_REAL_PLIST at a plist
// file (extract the <payloads> content of a GET response) and ZZ_REAL_KEYS at a
// comma-separated list of the keys you injected into.
func TestProbeStorageCategoriesFromRealPayload(t *testing.T) {
	path := os.Getenv("ZZ_REAL_PLIST")
	if path == "" {
		t.Skip("set ZZ_REAL_PLIST (and ZZ_REAL_KEYS) to classify a type from a real payload")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	keys := strings.Split(os.Getenv("ZZ_REAL_KEYS"), ",")
	if len(keys) == 0 || keys[0] == "" {
		t.Fatal("set ZZ_REAL_KEYS to the key(s) you injected hazard characters into")
	}

	platform := PlatformMacOS
	if strings.EqualFold(os.Getenv("ZZ_REAL_PLATFORM"), "mobile") {
		platform = PlatformMobileDevice
	}

	c := probeClient(t)
	stored, ok := roundTripProbe(t, c, platform, "ZZ real-payload storage probe", string(body))
	if !ok {
		t.Fatal("server refused even the real payload — check the injected values are valid for this type")
	}

	authoredTree, _, aErr := plisthelpers.ParsePlist(body)
	storedTree, _, sErr := plisthelpers.ParsePlist(stored)
	if aErr != nil || sErr != nil {
		t.Fatalf("parse: authored=%v stored=%v", aErr, sErr)
	}
	authoredFlat, storedFlat := map[string]string{}, map[string]string{}
	flattenStringLeaves("", authoredTree, authoredFlat)
	flattenStringLeaves("", storedTree, storedFlat)

	for _, k := range keys {
		k = strings.TrimSpace(k)
		ap, found := findLeafByKey(authoredFlat, k)
		if !found {
			t.Errorf("%s: not present in the authored payload", k)
			continue
		}
		got, present := "", false
		if sp, ok := findLeafByKey(storedFlat, k); ok {
			got, present = storedFlat[sp], true
		}
		t.Logf("%-42s %s", k, classifyProbeValue(authoredFlat[ap], got, present))
	}
}
