// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"slices"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// The custom_declarations schema validates `channel` and `kind` against an
// explicit list of SDK enum constants rather than against XxxValues(). That is
// deliberate: passing Values() through would silently widen the accepted set on
// an SDK bump, while the attribute's MarkdownDescription still enumerated the old
// values in prose — validation and documentation would disagree with nobody
// noticing.
//
// The cost of being explicit is that a value Jamf ADDS goes unnoticed instead.
// These tests are the tripwire for that: if the SDK enum grows, they fail and the
// maintainer must decide whether to accept the new value and update both the
// validator and the description.
func TestDeclarationChannelTypeEnum_HasNotGrown(t *testing.T) {
	want := map[string]bool{
		blueprints.DeclarationChannelTypeSystem: true,
		blueprints.DeclarationChannelTypeUser:   true,
	}
	assertEnumUnchanged(t, "DeclarationChannelType", want, blueprints.DeclarationChannelTypeValues())
}

func TestDeclarationKindEnum_HasNotGrown(t *testing.T) {
	want := map[string]bool{
		blueprints.DeclarationKindConfiguration: true,
		blueprints.DeclarationKindAsset:         true,
	}
	assertEnumUnchanged(t, "DeclarationKind", want, blueprints.DeclarationKindValues())
}

// assertEnumUnchanged fails when the SDK enum no longer matches the set the
// schema validates against, naming the difference in both directions.
func assertEnumUnchanged(t *testing.T, name string, want map[string]bool, got []string) {
	t.Helper()
	for _, v := range got {
		if !want[v] {
			t.Errorf("%s gained value %q: add it to the schema validator and to the attribute description, or state why it is excluded", name, v)
		}
	}
	if len(got) != len(want) {
		for v := range want {
			if !slices.Contains(got, v) {
				t.Errorf("%s no longer contains %q, which the schema validator still accepts", name, v)
			}
		}
	}
}
