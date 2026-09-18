// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBaseline writes a table to a temporary file, standing in for the committed artefact a
// regeneration is about to replace.
func writeBaseline(t *testing.T, name string, value any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding baseline: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("writing baseline: %v", err)
	}
	return path
}

func TestCompareTablesReportsWhatARegenerationDrops(t *testing.T) {
	previousProfiles := &table{Payloads: map[string]*payload{
		"com.apple.gone": {Keys: map[string]*schema{"Anything": {Type: "string"}}},
		"com.apple.kept": {Keys: map[string]*schema{
			"Stays": {Type: "string"},
			"Goes":  {Type: "string"},
			"Nested": {Type: typeDictionary, Keys: map[string]*schema{
				"Inner": {Type: "string"},
			}},
			"Items": {Type: typeArray, Item: &schema{Type: typeDictionary, Keys: map[string]*schema{
				"Element": {Type: "string"},
			}}},
		}},
	}}
	currentProfiles := &table{Payloads: map[string]*payload{
		"com.apple.kept": {Keys: map[string]*schema{
			"Stays":  {Type: "string"},
			"Nested": {Type: typeDictionary, Keys: map[string]*schema{}},
			"Items":  {Type: typeArray, Item: &schema{Type: typeDictionary, Keys: map[string]*schema{}}},
		}},
	}}

	previousDeclarations := &declarationTable{
		Declarations: map[string]*declaration{
			"com.apple.configuration.gone": {Keys: map[string]*schema{}},
			"com.apple.configuration.kept": {Keys: map[string]*schema{"Dropped": {Type: "string"}}},
		},
		StatusItems: []string{"device.identifier.serial-number", "device.gone"},
	}
	currentDeclarations := &declarationTable{
		Declarations: map[string]*declaration{
			"com.apple.configuration.kept": {Keys: map[string]*schema{}},
		},
		StatusItems: []string{"device.identifier.serial-number"},
	}

	found, err := compareTables(
		writeBaseline(t, "profiles.json", previousProfiles), currentProfiles,
		writeBaseline(t, "declarations.json", previousDeclarations), currentDeclarations,
	)
	if err != nil {
		t.Fatalf("compareTables: %v", err)
	}

	want := []string{
		"declaration com.apple.configuration.kept key Dropped",
		"declaration type com.apple.configuration.gone",
		"payload com.apple.kept key Goes",
		"payload com.apple.kept.Items[] key Element",
		"payload com.apple.kept.Nested key Inner",
		"payload type com.apple.gone",
		"status item device.gone",
	}
	if strings.Join(found.Findings, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(found.Findings, "\n"), strings.Join(want, "\n"))
	}
}

// A payload type that disappears is reported once, rather than once per key it carried: the type
// itself is the finding, and enumerating its keys would bury it.
func TestCompareTablesReportsARemovedTypeOnce(t *testing.T) {
	previous := &table{Payloads: map[string]*payload{
		"com.apple.gone": {Keys: map[string]*schema{"One": {Type: "string"}, "Two": {Type: "string"}}},
	}}

	found, err := compareTables(
		writeBaseline(t, "profiles.json", previous), &table{Payloads: map[string]*payload{}},
		filepath.Join(t.TempDir(), "absent.json"), &declarationTable{},
	)
	if err != nil {
		t.Fatalf("compareTables: %v", err)
	}
	if len(found.Findings) != 1 || found.Findings[0] != "payload type com.apple.gone" {
		t.Fatalf("findings: %v", found.Findings)
	}
}

// A first generation has no baseline on disk, which is not a removal.
func TestCompareTablesAcceptsAMissingBaseline(t *testing.T) {
	directory := t.TempDir()
	found, err := compareTables(
		filepath.Join(directory, "profiles.json"), &table{Payloads: map[string]*payload{}},
		filepath.Join(directory, "declarations.json"), &declarationTable{},
	)
	if err != nil {
		t.Fatalf("compareTables: %v", err)
	}
	if len(found.Findings) != 0 {
		t.Fatalf("findings: %v", found.Findings)
	}
}

// An unreadable baseline is an error, never silent agreement: a baseline that decodes as empty
// makes every regeneration look safe.
func TestCompareTablesRefusesAnUnreadableBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("writing baseline: %v", err)
	}
	if _, err := compareTables(path, &table{}, filepath.Join(t.TempDir(), "absent.json"), &declarationTable{}); err == nil {
		t.Fatal("expected an error for an undecodable baseline")
	}
}

func TestRemovalReportElidesBeyondTheBound(t *testing.T) {
	found := &removals{}
	for i := 0; i < maxReportedRemovals+3; i++ {
		found.record("payload type com.apple.%d", i)
	}
	report := found.report()
	if strings.Count(report, "\n") != maxReportedRemovals+1 {
		t.Fatalf("report lines:\n%s", report)
	}
	if !strings.Contains(report, "and 3 more") {
		t.Fatalf("report does not say how many were elided:\n%s", report)
	}
}
