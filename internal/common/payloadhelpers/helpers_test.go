// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package payloadhelpers

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/plisthelpers"
)

const minimalPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadUUID</key><string>AAAABBBB-1111-2222-3333-444455556666</string>
<key>PayloadIdentifier</key><string>AAAABBBB-1111-2222-3333-444455556666</string>
<key>PayloadDisplayName</key><string>Test Profile</string>
<key>PayloadContent</key><array/>
</dict></plist>`

const minimalWireXML = `<?xml version="1.0" encoding="UTF-8"?>
<configuration_profile><general><id>1</id><payloads>&lt;?xml version=&quot;1.0&quot; encoding=&quot;UTF-8&quot;?&gt;
&lt;!DOCTYPE plist PUBLIC &quot;-//Apple//DTD PLIST 1.0//EN&quot; &quot;http://www.apple.com/DTDs/PropertyList-1.0.dtd&quot;&gt;
&lt;plist version=&quot;1.0&quot;&gt;&lt;dict&gt;
&lt;key&gt;PayloadType&lt;/key&gt;&lt;string&gt;Configuration&lt;/string&gt;
&lt;key&gt;PayloadVersion&lt;/key&gt;&lt;integer&gt;1&lt;/integer&gt;
&lt;key&gt;PayloadUUID&lt;/key&gt;&lt;string&gt;AAAABBBB-1111-2222-3333-444455556666&lt;/string&gt;
&lt;key&gt;PayloadIdentifier&lt;/key&gt;&lt;string&gt;AAAABBBB-1111-2222-3333-444455556666&lt;/string&gt;
&lt;key&gt;PayloadDisplayName&lt;/key&gt;&lt;string&gt;Test Profile&lt;/string&gt;
&lt;key&gt;PayloadContent&lt;/key&gt;&lt;array/&gt;
&lt;/dict&gt;&lt;/plist&gt;</payloads></general></configuration_profile>`

func TestPayloadsSemanticallyEqual_Identical(t *testing.T) {
	eq, err := PayloadsSemanticallyEqual([]byte(minimalPlist), []byte(minimalPlist))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Fatal("identical payloads must be equal")
	}
}

func TestPayloadsSemanticallyEqual_MaskedKeysOnly_Equal(t *testing.T) {
	const a = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>PayloadType</key><string>Configuration</string><key>PayloadVersion</key><integer>1</integer><key>PayloadUUID</key><string>AAAA</string><key>PayloadDisplayName</key><string>A</string><key>PayloadContent</key><array/></dict></plist>`
	const b = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>PayloadType</key><string>Configuration</string><key>PayloadVersion</key><integer>1</integer><key>PayloadUUID</key><string>BBBB</string><key>PayloadDisplayName</key><string>B</string><key>PayloadContent</key><array/></dict></plist>`
	eq, err := PayloadsSemanticallyEqual([]byte(a), []byte(b))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Fatal("diff only in masked keys must be equal")
	}
}

func TestPayloadsSemanticallyEqual_RealChange_NotEqual(t *testing.T) {
	const a = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>PayloadType</key><string>Configuration</string><key>PayloadVersion</key><integer>1</integer><key>PayloadContent</key><array/></dict></plist>`
	const b = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>PayloadType</key><string>Configuration</string><key>PayloadVersion</key><integer>2</integer><key>PayloadContent</key><array/></dict></plist>`
	eq, err := PayloadsSemanticallyEqual([]byte(a), []byte(b))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if eq {
		t.Fatal("changed PayloadVersion must not be equal")
	}
}

func TestPayloadsSemanticallyEqual_WhitespaceDiffEqual(t *testing.T) {
	reformatted := strings.ReplaceAll(minimalPlist, "\t", "    ")
	eq, err := PayloadsSemanticallyEqual([]byte(minimalPlist), []byte(reformatted))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Fatal("whitespace-only diff must be equal")
	}
}

func TestInjectTopLevelIdentifierValues_ReplacesUUIDs(t *testing.T) {
	const existingUUID = "CCCCDDDD-5555-6666-7777-888899990000"
	out, err := InjectTopLevelIdentifierValues([]byte(minimalPlist), existingUUID, existingUUID)
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	check, _, err := plisthelpers.ParsePlist(out)
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if check["PayloadUUID"] != existingUUID {
		t.Errorf("PayloadUUID: got=%v want=%v", check["PayloadUUID"], existingUUID)
	}
	if check["PayloadIdentifier"] != existingUUID {
		t.Errorf("PayloadIdentifier: got=%v want=%v", check["PayloadIdentifier"], existingUUID)
	}
}

func TestStripTopLevelIdentifiers_RemovesTopLevelOnly(t *testing.T) {
	const nested = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadUUID</key><string>TOP-UUID</string>
<key>PayloadIdentifier</key><string>TOP-ID</string>
<key>PayloadContent</key><array><dict>
<key>PayloadType</key><string>com.apple.dock</string>
<key>PayloadUUID</key><string>INNER-UUID</string>
<key>PayloadIdentifier</key><string>INNER-ID</string>
</dict></array>
</dict></plist>`
	out, err := StripTopLevelIdentifiers([]byte(nested))
	if err != nil {
		t.Fatalf("strip: %v", err)
	}
	got, _, err := plisthelpers.ParsePlist(out)
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if _, ok := got["PayloadUUID"]; ok {
		t.Error("top-level PayloadUUID should be removed")
	}
	if _, ok := got["PayloadIdentifier"]; ok {
		t.Error("top-level PayloadIdentifier should be removed")
	}
	inner, _ := got["PayloadContent"].([]any)
	if len(inner) != 1 {
		t.Fatalf("PayloadContent: got %d entries want 1", len(inner))
	}
	entry, _ := inner[0].(map[string]any)
	if entry["PayloadUUID"] != "INNER-UUID" || entry["PayloadIdentifier"] != "INNER-ID" {
		t.Errorf("nested identifiers must be untouched: %v", entry)
	}
}

func TestStripTopLevelIdentifiers_AbsentIsNoOp(t *testing.T) {
	const bare = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>PayloadType</key><string>Configuration</string></dict></plist>`
	out, err := StripTopLevelIdentifiers([]byte(bare))
	if err != nil {
		t.Fatalf("strip: %v", err)
	}
	if string(out) != bare {
		t.Errorf("payload without top-level identifiers must come back byte-identical; got %s", out)
	}
}

func TestStripTopLevelIdentifiers_RejectsMalformed(t *testing.T) {
	if _, err := StripTopLevelIdentifiers([]byte("not a plist")); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestInjectTopLevelIdentifierValues_EmptyUUID_NoOp(t *testing.T) {
	out, err := InjectTopLevelIdentifierValues([]byte(minimalPlist), "", "")
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	if string(out) != minimalPlist {
		t.Fatal("expected no-op when both uuid and identifier are empty")
	}
}

func TestInjectTopLevelIdentifiers_NilExisting_NoOp(t *testing.T) {
	in := []byte(minimalPlist)
	out, err := InjectTopLevelIdentifiers(in, nil)
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	if string(out) != string(in) {
		t.Fatal("expected no-op when existing is nil")
	}
}

func TestInjectTopLevelIdentifiers_UnparseableExisting_NoOp(t *testing.T) {
	in := []byte(minimalPlist)
	out, err := InjectTopLevelIdentifiers(in, []byte("not a plist"))
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	if string(out) != string(in) {
		t.Fatal("expected no-op when existing is unparseable")
	}
}

func TestExtractServerPayloadFromGeneral_DecodesEntities(t *testing.T) {
	payload, err := ExtractServerPayloadFromGeneral([]byte(minimalWireXML))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if _, _, err := plisthelpers.ParsePlist(payload); err != nil {
		t.Fatalf("extracted bytes not valid plist: %v", err)
	}
}

func TestExtractServerPayloadFromGeneral_NoPayloadsBlock(t *testing.T) {
	_, err := ExtractServerPayloadFromGeneral([]byte(`<configuration_profile><general><id>1</id></general></configuration_profile>`))
	if err == nil {
		t.Fatal("expected error when no <payloads> block")
	}
}

func TestMaskPayload_DropsTopLevelMaskedKeys(t *testing.T) {
	m, err := MaskPayload([]byte(minimalPlist))
	if err != nil {
		t.Fatalf("mask: %v", err)
	}
	for _, k := range []string{"PayloadUUID", "PayloadIdentifier", "PayloadDisplayName"} {
		if _, ok := m[k]; ok {
			t.Errorf("masked key %q must be dropped", k)
		}
	}
}

func TestMaskPayload_DropsServerInjectedPayloadTypes(t *testing.T) {
	// Plan side has one PayloadContent entry (notifications). Server side
	// echoes back the same entry plus a server-injected
	// com.apple.profileRemovalPassword entry that materialised because the
	// user set the classic-API authorization_password field. After masking,
	// both sides must have the same single-entry PayloadContent array so
	// PayloadsSemanticallyEqual returns true.
	plan := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadType</key><string>Configuration</string>
  <key>PayloadIdentifier</key><string>plan-id</string>
  <key>PayloadUUID</key><string>plan-uuid</string>
  <key>PayloadVersion</key><integer>1</integer>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>PayloadType</key><string>com.apple.notificationsettings</string>
      <key>PayloadUUID</key><string>nf-uuid</string>
      <key>PayloadIdentifier</key><string>nf-id</string>
      <key>PayloadVersion</key><integer>1</integer>
    </dict>
  </array>
</dict>
</plist>`
	server := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadType</key><string>Configuration</string>
  <key>PayloadIdentifier</key><string>server-id</string>
  <key>PayloadUUID</key><string>server-uuid</string>
  <key>PayloadVersion</key><integer>1</integer>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>PayloadType</key><string>com.apple.profileRemovalPassword</string>
      <key>PayloadUUID</key><string>rp-uuid</string>
      <key>PayloadIdentifier</key><string>rp-id</string>
      <key>PayloadVersion</key><integer>1</integer>
      <key>RemovalPassword</key><string>secret</string>
    </dict>
    <dict>
      <key>PayloadType</key><string>com.apple.notificationsettings</string>
      <key>PayloadUUID</key><string>nf-uuid</string>
      <key>PayloadIdentifier</key><string>nf-id</string>
      <key>PayloadVersion</key><integer>1</integer>
    </dict>
  </array>
</dict>
</plist>`
	eq, err := PayloadsSemanticallyEqual([]byte(plan), []byte(server))
	if err != nil {
		t.Fatalf("PayloadsSemanticallyEqual: %v", err)
	}
	if !eq {
		t.Fatal("expected plan and server payloads to be semantically equal after server-injected RemovalPassword filtered out")
	}
}

func TestLenientEqualPlist_AsymmetricKey_Ignored(t *testing.T) {
	a := map[string]any{"shared": "value", "onlyInA": "x"}
	b := map[string]any{"shared": "value"}
	if !LenientEqualPlist(a, b) {
		t.Fatal("asymmetric key must be ignored (intersection semantics)")
	}
}

func TestLenientEqualPlist_SharedKeyDiffers_NotEqual(t *testing.T) {
	a := map[string]any{"k": "v1"}
	b := map[string]any{"k": "v2"}
	if LenientEqualPlist(a, b) {
		t.Fatal("differing shared key must not be equal")
	}
}

// MCX drift-detection fixtures: a minimal "Application & Custom Settings"
// payload shaped like local-testing/pro/support_files/minimal_notifications.mobileconfig.
// The inner mcx_preference_settings dict is opaque user-content; key add/remove
// inside it must surface as drift.
const mcxBaseline = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>top-id</string>
<key>PayloadUUID</key><string>top-uuid</string>
<key>PayloadContent</key><array><dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>mcx-id</string>
<key>PayloadUUID</key><string>mcx-uuid</string>
<key>PayloadContent</key><dict>
<key>com.example.app</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict>
<key>FeatureEnabled</key><true/>
<key>BannerMessage</key><string>hello</string>
</dict>
</dict></array>
</dict>
</dict>
</dict></array>
</dict></plist>`

const mcxAddedInnerKey = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>top-id</string>
<key>PayloadUUID</key><string>top-uuid</string>
<key>PayloadContent</key><array><dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>mcx-id</string>
<key>PayloadUUID</key><string>mcx-uuid</string>
<key>PayloadContent</key><dict>
<key>com.example.app</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict>
<key>FeatureEnabled</key><true/>
<key>BannerMessage</key><string>hello</string>
<key>EnablePageZeroProtection2</key><false/>
</dict>
</dict></array>
</dict>
</dict>
</dict></array>
</dict></plist>`

const mcxRemovedInnerKey = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>top-id</string>
<key>PayloadUUID</key><string>top-uuid</string>
<key>PayloadContent</key><array><dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>mcx-id</string>
<key>PayloadUUID</key><string>mcx-uuid</string>
<key>PayloadContent</key><dict>
<key>com.example.app</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict>
<key>FeatureEnabled</key><true/>
</dict>
</dict></array>
</dict>
</dict>
</dict></array>
</dict></plist>`

const mcxChangedInnerValue = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>top-id</string>
<key>PayloadUUID</key><string>top-uuid</string>
<key>PayloadContent</key><array><dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>mcx-id</string>
<key>PayloadUUID</key><string>mcx-uuid</string>
<key>PayloadContent</key><dict>
<key>com.example.app</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict>
<key>FeatureEnabled</key><false/>
<key>BannerMessage</key><string>hello</string>
</dict>
</dict></array>
</dict>
</dict>
</dict></array>
</dict></plist>`

// PayloadContent reorder fixtures: wire-observed 2026-08-11 against a live
// Jamf Pro 11.30.x tenant, a profile mixing an MCX "Application & Custom
// Settings" entry with a Certificate entry comes back with the certificate
// moved ahead of MCX. A positional array compare pairs the wrong entries
// (MCX vs certificate) and reports cascading false drift across every key —
// this is the root cause of the original "Jamf Pro cannot store this payload
// faithfully" false positive.
const mcxThenCert = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>top-id</string>
<key>PayloadUUID</key><string>top-uuid</string>
<key>PayloadContent</key><array>
<dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>mcx-id</string>
<key>PayloadUUID</key><string>mcx-uuid</string>
<key>PayloadContent</key><dict>
<key>com.example.app</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict>
<key>FeatureEnabled</key><true/>
<key>BannerMessage</key><string>hello</string>
</dict>
</dict></array>
</dict>
</dict>
</dict>
<dict>
<key>PayloadType</key><string>com.apple.security.root</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>cert-id</string>
<key>PayloadUUID</key><string>cert-uuid</string>
<key>PayloadCertificateFileName</key><string>Test.crt</string>
<key>PayloadContent</key><data>AAAA</data>
</dict>
</array>
</dict></plist>`

const certThenMCXSameContent = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>top-id</string>
<key>PayloadUUID</key><string>top-uuid</string>
<key>PayloadContent</key><array>
<dict>
<key>PayloadType</key><string>com.apple.security.root</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>cert-id</string>
<key>PayloadUUID</key><string>cert-uuid</string>
<key>PayloadCertificateFileName</key><string>Test.crt</string>
<key>PayloadContent</key><data>AAAA</data>
</dict>
<dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>mcx-id</string>
<key>PayloadUUID</key><string>mcx-uuid</string>
<key>PayloadContent</key><dict>
<key>com.example.app</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict>
<key>FeatureEnabled</key><true/>
<key>BannerMessage</key><string>hello</string>
</dict>
</dict></array>
</dict>
</dict>
</dict>
</array>
</dict></plist>`

const certThenMCXChangedInnerValue = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>top-id</string>
<key>PayloadUUID</key><string>top-uuid</string>
<key>PayloadContent</key><array>
<dict>
<key>PayloadType</key><string>com.apple.security.root</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>cert-id</string>
<key>PayloadUUID</key><string>cert-uuid</string>
<key>PayloadCertificateFileName</key><string>Test.crt</string>
<key>PayloadContent</key><data>AAAA</data>
</dict>
<dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>mcx-id</string>
<key>PayloadUUID</key><string>mcx-uuid</string>
<key>PayloadContent</key><dict>
<key>com.example.app</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict>
<key>FeatureEnabled</key><false/>
<key>BannerMessage</key><string>hello</string>
</dict>
</dict></array>
</dict>
</dict>
</dict>
</array>
</dict></plist>`

func TestPayloadsSemanticallyEqual_PayloadContentReordered_Equal(t *testing.T) {
	eq, err := PayloadsSemanticallyEqual([]byte(mcxThenCert), []byte(certThenMCXSameContent))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Fatal("Jamf Pro reordering PayloadContent entries on store must not report drift")
	}
}

func TestPayloadsSemanticallyEqual_PayloadContentReorderedWithRealDrift_NotEqual(t *testing.T) {
	eq, err := PayloadsSemanticallyEqual([]byte(mcxThenCert), []byte(certThenMCXChangedInnerValue))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if eq {
		t.Fatal("a real change inside the reordered MCX entry must still be reported as drift")
	}
}

func TestPayloadsSemanticallyEqual_MCXIdentical_Equal(t *testing.T) {
	eq, err := PayloadsSemanticallyEqual([]byte(mcxBaseline), []byte(mcxBaseline))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Fatal("identical MCX payloads must compare equal")
	}
}

func TestPayloadsSemanticallyEqual_MCXAddedInnerKey_NotEqual(t *testing.T) {
	eq, err := PayloadsSemanticallyEqual([]byte(mcxBaseline), []byte(mcxAddedInnerKey))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if eq {
		t.Fatal("admin-injected key inside mcx_preference_settings must produce drift")
	}
}

func TestPayloadsSemanticallyEqual_MCXRemovedInnerKey_NotEqual(t *testing.T) {
	eq, err := PayloadsSemanticallyEqual([]byte(mcxBaseline), []byte(mcxRemovedInnerKey))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if eq {
		t.Fatal("admin-removed key inside mcx_preference_settings must produce drift")
	}
}

func TestPayloadsSemanticallyEqual_MCXChangedInnerValue_NotEqual(t *testing.T) {
	eq, err := PayloadsSemanticallyEqual([]byte(mcxBaseline), []byte(mcxChangedInnerValue))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if eq {
		t.Fatal("admin-changed value inside mcx_preference_settings must produce drift")
	}
}

// Base64-in-string normalization: when a vendor payload embeds a base64 blob
// inside a <string> value and Jamf Pro line-wraps it on read, the whitespace
// difference must not produce a spurious diff.
const base64Compact = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>EmbeddedCert</key><string>TUlJRENDQWZpZ0F3SUJBZ0lVTUJ4UkxEdGRCZW9PNkZ3MGZQMzZ5cFppL0VBd0RRWUpLb1pJaHZjTkFRRUw=</string>
<key>PayloadContent</key><array/>
</dict></plist>`

const base64Wrapped = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>EmbeddedCert</key><string>TUlJRENDQWZpZ0F3SUJBZ0lVTUJ4
UkxEdGRCZW9PNkZ3MGZQMzZ5cFpp
L0VBd0RRWUpLb1pJaHZjTkFRRUw=</string>
<key>PayloadContent</key><array/>
</dict></plist>`

func TestPayloadsSemanticallyEqual_Base64InString_WrappedAndCompactEqual(t *testing.T) {
	eq, err := PayloadsSemanticallyEqual([]byte(base64Compact), []byte(base64Wrapped))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Fatal("base64-in-string differing only by line-wrap whitespace must compare equal")
	}
}

func TestNormalizeBase64InString_NonBase64Unchanged(t *testing.T) {
	in := "Allow 1Password Launch Item"
	if got := normalizeBase64InString(in); got != in {
		t.Errorf("non-base64 text mutated: got %q want %q", got, in)
	}
}

func TestNormalizeBase64InString_NoWhitespaceUnchanged(t *testing.T) {
	in := "TUlJRENDQWZpZw=="
	if got := normalizeBase64InString(in); got != in {
		t.Errorf("base64 without whitespace mutated: got %q want %q", got, in)
	}
}

func TestNormalizeBase64InString_StripsInternalWhitespace(t *testing.T) {
	in := "TUlJRENDQWZpZ0F3SUJBZ0lVTUJ4\nUkxEdGRCZW9PNkZ3MGZQMzZ5cFppL0VBd0RRWUpLb1pJaHZjTkFRRUw="
	want := "TUlJRENDQWZpZ0F3SUJBZ0lVTUJ4UkxEdGRCZW9PNkZ3MGZQMzZ5cFppL0VBd0RRWUpLb1pJaHZjTkFRRUw="
	if got := normalizeBase64InString(in); got != want {
		t.Errorf("base64 with whitespace not collapsed: got %q want %q", got, want)
	}
}

func TestNormalizeBase64InString_EmptyAfterStripUnchanged(t *testing.T) {
	in := "   \n\t  "
	if got := normalizeBase64InString(in); got != in {
		t.Errorf("whitespace-only string mutated: got %q want %q", got, in)
	}
}

func TestNormalizeBase64InString_NaturalTextWithSpacesUnchanged(t *testing.T) {
	// Natural-language text containing only base64-alphabet characters and
	// spaces — guards against the heuristic over-firing on values like
	// "Allow 1Password Launch Item" whose whitespace-stripped form would
	// coincidentally decode as base64.
	in := "Allow 1Password Launch Item"
	if got := normalizeBase64InString(in); got != in {
		t.Errorf("natural text with spaces mutated: got %q want %q", got, in)
	}
}

func TestNormalizeBase64InString_NewlineButTooShortUnchanged(t *testing.T) {
	// Newline present but stripped form is below the 32-char floor — must
	// not be collapsed even if it happens to decode as base64.
	in := "TUlJ\nRENDQQ=="
	if got := normalizeBase64InString(in); got != in {
		t.Errorf("short newline-bearing string mutated: got %q want %q", got, in)
	}
}

func TestNormalizeBase64InString_NewlineNotBase64Unchanged(t *testing.T) {
	// Multi-line natural-language string that does not decode as base64 —
	// must pass through untouched even though it has newlines.
	in := "This is a multi-line description\nthat continues onto a second line\nand even a third for good measure."
	if got := normalizeBase64InString(in); got != in {
		t.Errorf("multi-line non-base64 text mutated: got %q want %q", got, in)
	}
}

func TestPrepareWirePayload_CreatePathCompactsOnly(t *testing.T) {
	// Create path: empty identifiers skip injection, but structural
	// whitespace still gets compacted before send.
	in := "<plist version=\"1.0\">\n<dict>\n\t<key>Pages</key>\n\t<array>\n\t\t<array/>\n\t</array>\n</dict>\n</plist>\n"
	want := `<plist version="1.0"><dict><key>Pages</key><array><array/></array></dict></plist>`
	got, err := PrepareWirePayload([]byte(in), "", "")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if string(got) != want {
		t.Errorf("create-path compaction:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestPrepareWirePayload_UpdatePathInjectsAndCompacts(t *testing.T) {
	in := "<dict>\n\t<key>PayloadUUID</key>\n\t<string>OLD</string>\n\t<key>A</key>\n\t<array>\n\t\t<array/>\n\t</array>\n</dict>\n"
	got, err := PrepareWirePayload([]byte(in), "server-uuid", "server-id")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	parsed, _, err := plisthelpers.ParsePlist(got)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if parsed["PayloadUUID"] != "server-uuid" || parsed["PayloadIdentifier"] != "server-id" {
		t.Errorf("identifiers not injected: %v / %v", parsed["PayloadUUID"], parsed["PayloadIdentifier"])
	}
	// Injection re-serialises pretty-printed; compaction must have removed
	// every structural gap the re-serialise introduced.
	if bytes.Contains(got, []byte(">\n")) || bytes.Contains(got, []byte("\t<")) {
		t.Errorf("structural whitespace survived: %s", got)
	}
}

func TestPrepareWirePayload_MalformedCreatePassesThrough(t *testing.T) {
	// Create path with non-XML content: injection is skipped (empty ids) and
	// compaction falls back to the original bytes — server reports the
	// malformation.
	in := []byte("<dict><key>unclosed")
	got, err := PrepareWirePayload(in, "", "")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !bytes.Equal(got, in) {
		t.Errorf("malformed create payload mutated: got %q", got)
	}
}

// Line-break fixtures. A carriage return can only be authored as a `&#13;`
// character reference (XML 1.0 §2.11 turns a literal CR into LF in transit),
// and CR is the only whitespace character Jamf Pro keeps inside string values
// — literal LF and TAB are deleted outright by the payload types it stores
// verbatim. Both profile endpoints nevertheless hand the value back as LF:
// MCX custom settings and mobile payload fragments normalise CR→LF when
// storing, and a verbatim-stored CR comes back as a raw CR byte our own parse
// then normalises. So the authored side holds CR, the server side LF, and the
// mask must make the two compare equal or every `&#13;` reads as an
// unfaithful store.

// mcxWithBanner substitutes the MCX inner BannerMessage value. That string
// lives inside mcx_preference_settings, which LenientEqualPlist strict-compares
// — the tolerance has to hold on the strict path too, not just the
// intersection one.
func mcxWithBanner(t *testing.T, banner string) []byte {
	t.Helper()
	out := strings.Replace(mcxBaseline, "<string>hello</string>", "<string>"+banner+"</string>", 1)
	if out == mcxBaseline {
		t.Fatal("banner substitution did not apply — mcxBaseline fixture changed")
	}
	return []byte(out)
}

// consentTextTemplate carries a multi-line value outside PayloadContent, the
// other slot where Jamf Pro deletes literal LF (top-level ConsentText is an
// Apple-documented key mSCP emits into every generated profile).
const consentTextTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>ConsentText</key><dict><key>default</key><string>BANNER</string></dict>
<key>PayloadContent</key><array/>
</dict></plist>`

func consentTextWith(t *testing.T, banner string) []byte {
	t.Helper()
	out := strings.Replace(consentTextTemplate, "BANNER", banner, 1)
	if out == consentTextTemplate {
		t.Fatal("banner substitution did not apply — consentTextTemplate fixture changed")
	}
	return []byte(out)
}

// TestParsePlist_CarriageReturnRefParsesToCR pins the premise the whole
// tolerance rests on: the plist parse keeps a `&#13;` reference as a real CR
// (character references are exempt from XML line-end normalisation, which only
// rewrites literal CR bytes). If a parser upgrade ever normalised the reference
// to LF, the mask work below would be dead code and this fails first.
func TestParsePlist_CarriageReturnRefParsesToCR(t *testing.T) {
	parsed, _, err := plisthelpers.ParsePlist(consentTextWith(t, "line one&#13;line two"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	consent, ok := parsed["ConsentText"].(map[string]any)
	if !ok {
		t.Fatalf("ConsentText missing or wrong type: %T", parsed["ConsentText"])
	}
	got, _ := consent["default"].(string)
	if got != "line one\rline two" {
		t.Errorf("CR reference did not parse to a carriage return: got %q", got)
	}
}

func TestPayloadsSemanticallyEqual_MCXCarriageReturnRefVsStoredLF_Equal(t *testing.T) {
	authored := mcxWithBanner(t, "line one&#13;line two")
	stored := mcxWithBanner(t, "line one\nline two")
	eq, err := PayloadsSemanticallyEqual(authored, stored)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Fatal("authored CR reference must compare equal to the LF Jamf Pro stores for MCX payloads")
	}
}

func TestPayloadsSemanticallyEqual_MCXCRLFRefVsStoredLF_Equal(t *testing.T) {
	// `&#13;&#10;` is the CRLF form Jamf's own UI emits; it must collapse to
	// one LF, not two.
	authored := mcxWithBanner(t, "line one&#13;&#10;line two")
	stored := mcxWithBanner(t, "line one\nline two")
	eq, err := PayloadsSemanticallyEqual(authored, stored)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Fatal("authored CRLF reference must collapse to a single LF")
	}
}

func TestPayloadsStructurallyEqual_CarriageReturnRefVsStoredLF_Equal(t *testing.T) {
	// The three-way comparator masks through the same helper, so the
	// tolerance must hold on the strict structural path used for
	// last-canonical-vs-server drift detection.
	authored := consentTextWith(t, "line one&#13;line two")
	stored := consentTextWith(t, "line one\nline two")
	eq, err := PayloadsStructurallyEqual(authored, stored)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Fatal("CR-vs-LF must not register as structural drift")
	}
}

func TestPayloadsSemanticallyEqual_LineBreakDeleted_NotEqual(t *testing.T) {
	// The failure the SDK CR fix exists to prevent: the server dropping the
	// break entirely and merging the words. Must still surface.
	authored := consentTextWith(t, "line one&#13;line two")
	stored := consentTextWith(t, "line oneline two")
	eq, err := PayloadsSemanticallyEqual(authored, stored)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if eq {
		t.Fatal("a deleted line break must not be tolerated")
	}
}

func TestPayloadsSemanticallyEqual_LineSeparatorNotNormalised_NotEqual(t *testing.T) {
	// U+2028 round-trips byte-exact through every slot, so it keeps full
	// drift fidelity — normalising it would give up detection for nothing.
	authored := consentTextWith(t, "line one&#8232;line two")
	stored := consentTextWith(t, "line one\nline two")
	eq, err := PayloadsSemanticallyEqual(authored, stored)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if eq {
		t.Fatal("U+2028 must stay distinct from LF")
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"no carriage return untouched", "line one\nline two", "line one\nline two"},
		{"lone CR", "line one\rline two", "line one\nline two"},
		{"CRLF collapses to one LF", "line one\r\nline two", "line one\nline two"},
		{"repeated CR", "a\r\rb", "a\n\nb"},
		{"CR then CRLF", "a\r\r\nb", "a\n\nb"},
		{"LFCR is two breaks", "a\n\rb", "a\n\nb"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeLineEndings(tc.in); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

// payloadContentTypeOrder returns the PayloadType of each top-level
// PayloadContent entry, in document order.
func payloadContentTypeOrder(t *testing.T, raw []byte) []string {
	t.Helper()
	tree, _, err := plisthelpers.ParsePlist(raw)
	if err != nil {
		t.Fatalf("parsing payload: %v", err)
	}
	arr, ok := tree["PayloadContent"].([]any)
	if !ok {
		t.Fatal("payload has no PayloadContent array")
	}
	out := make([]string, 0, len(arr))
	for _, entry := range arr {
		out = append(out, payloadTypeOf(entry))
	}
	return out
}

// TestPayloadsSemanticallyEqual_RecordedReorderedResponse_Equal is the
// wire-backed twin of the two hand-written reorder tests above: real authored
// bytes and the real response a live Jamf Pro returned for them, so it guards
// the actual server behaviour rather than our model of it.
//
// The fixture mixes an MCX "Application & Custom Settings" entry with a
// certificate entry — a certificate is stored verbatim while MCX is re-rendered,
// which is the condition that makes Jamf Pro reorder the array. Before the
// canonical ordering in maskPayloadContent this pair reported drift on every key
// of both entries, rolled back the freshly created profile, and told the operator
// Jamf Pro could not store a payload it had in fact stored perfectly.
func TestPayloadsSemanticallyEqual_RecordedReorderedResponse_Equal(t *testing.T) {
	const (
		fixtures = "../../resources/pro/macos_configuration_profile/testdata"
		stem     = "GoogleChrome__mcx_with_certificate_profile"
	)
	authored, err := os.ReadFile(filepath.Join(fixtures, stem+".mobileconfig"))
	if err != nil {
		t.Fatalf("reading authored fixture: %v", err)
	}
	resp, err := os.ReadFile(filepath.Join(fixtures, stem+".create_response.xml"))
	if err != nil {
		t.Fatalf("reading recorded response: %v", err)
	}
	stored, err := ExtractServerPayloadFromGeneral(resp)
	if err != nil {
		t.Fatalf("extracting payload from recorded response: %v", err)
	}

	// Guard the guard: if this tenant ever stops reordering, the assertions below
	// would pass for the wrong reason and quietly stop covering anything.
	authoredOrder := payloadContentTypeOrder(t, authored)
	storedOrder := payloadContentTypeOrder(t, stored)
	if strings.Join(authoredOrder, ",") == strings.Join(storedOrder, ",") {
		t.Fatalf("fixture no longer exhibits the reorder it exists to cover: %v", storedOrder)
	}

	eq, err := PayloadsSemanticallyEqual(authored, stored)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !eq {
		t.Errorf("recorded response reordered %v to %v and this must not read as drift", authoredOrder, storedOrder)
	}

	// The diagnostic path has to agree with the equality gate, or a profile that
	// compares equal would still produce a findings list if anything else failed.
	findings, ok := diffPayloadStrings(authored, stored)
	if !ok {
		t.Fatal("fixture pair did not parse for the fidelity diff")
	}
	if len(findings) != 0 {
		for _, f := range findings {
			t.Logf("unexpected finding: %s (present=%v) authored=%q stored=%q", f.path, f.present, f.authored, f.stored)
		}
		t.Errorf("recorded response stored every value faithfully, got %d fidelity finding(s)", len(findings))
	}
}

// TestPayloadsSemanticallyEqual_SameTypeSiblingDrift_NotEqual pins the limit of
// the canonical ordering: the sort is keyed only on PayloadType and is stable, so
// two entries of the same type keep their authored positions relative to each
// other and an edit to one of them is still drift. A non-stable sort, or one that
// also ordered by content, would silently swallow this.
func TestPayloadsSemanticallyEqual_SameTypeSiblingDrift_NotEqual(t *testing.T) {
	eq, err := PayloadsSemanticallyEqual([]byte(twoMCXSiblings), []byte(twoMCXSiblingsSecondEdited))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if eq {
		t.Fatal("an edit inside the second of two same-type siblings must surface as drift")
	}
}

const twoMCXSiblings = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>top-id</string>
<key>PayloadUUID</key><string>top-uuid</string>
<key>PayloadContent</key><array>
<dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadContent</key><dict>
<key>com.example.first</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict><key>Flag</key><true/></dict>
</dict></array>
</dict>
</dict>
</dict>
<dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadContent</key><dict>
<key>com.example.second</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict><key>Flag</key><true/></dict>
</dict></array>
</dict>
</dict>
</dict>
</array>
</dict></plist>`

const twoMCXSiblingsSecondEdited = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadIdentifier</key><string>top-id</string>
<key>PayloadUUID</key><string>top-uuid</string>
<key>PayloadContent</key><array>
<dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadContent</key><dict>
<key>com.example.first</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict><key>Flag</key><true/></dict>
</dict></array>
</dict>
</dict>
</dict>
<dict>
<key>PayloadType</key><string>com.apple.ManagedClient.preferences</string>
<key>PayloadVersion</key><integer>1</integer>
<key>PayloadContent</key><dict>
<key>com.example.second</key><dict>
<key>Forced</key><array><dict>
<key>mcx_preference_settings</key><dict><key>Flag</key><false/></dict>
</dict></array>
</dict>
</dict>
</dict>
</array>
</dict></plist>`
