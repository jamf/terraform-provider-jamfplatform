// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// maxReportedRemovals bounds the report. A handful of removals is a diff to read; hundreds means
// the generator read the wrong thing, and printing every one buries that under its own evidence.
const maxReportedRemovals = 25

// removals is what a regeneration takes away from the embedded tables: a name the provider
// recognises today and reports as unknown afterwards. The tables track upstream rather than
// retaining a vocabulary Apple has retired, so a withdrawal is carried through deliberately — but
// every finding the tables produce is an error, so it turns a configuration that plans today into
// one that fails. Reporting it is what keeps that visible, since a refresh changes several thousand
// lines and the withdrawn names are a handful among them.
type removals struct {
	Findings []string
}

// compareTables lists what the newly generated tables no longer carry, relative to the tables they
// are about to replace. It only reports: the generated tables are built from the checkouts alone
// and nothing here feeds back into them. Both baselines are optional — a first generation has
// nothing to compare against — while an unreadable one is an error rather than agreement, because a
// baseline that silently decodes as empty makes every regeneration look safe.
func compareTables(profilesPath string, profiles *table, declarationsPath string, declarations *declarationTable) (*removals, error) {
	found := &removals{}

	previousProfiles, err := readPrevious[table](profilesPath)
	if err != nil {
		return nil, err
	}
	if previousProfiles != nil {
		comparePayloads(found, previousProfiles, profiles)
	}

	previousDeclarations, err := readPrevious[declarationTable](declarationsPath)
	if err != nil {
		return nil, err
	}
	if previousDeclarations != nil {
		compareDeclarations(found, previousDeclarations, declarations)
	}

	sort.Strings(found.Findings)
	return found, nil
}

// readPrevious decodes a committed table, returning nil when the file does not exist yet.
func readPrevious[T any](path string) (*T, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var decoded T
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &decoded, nil
}

// comparePayloads records every payload type and payload key the regenerated profile table drops.
func comparePayloads(found *removals, previous, current *table) {
	for payloadType, was := range previous.Payloads {
		is, kept := current.Payloads[payloadType]
		if !kept {
			found.record("payload type %s", payloadType)
			continue
		}
		compareKeys(found, "payload "+payloadType, was.Keys, is.Keys)
	}
}

// compareDeclarations records every declaration type, declaration key and status item the
// regenerated declaration table drops.
func compareDeclarations(found *removals, previous, current *declarationTable) {
	for declarationType, was := range previous.Declarations {
		is, kept := current.Declarations[declarationType]
		if !kept {
			found.record("declaration type %s", declarationType)
			continue
		}
		compareKeys(found, "declaration "+declarationType, was.Keys, is.Keys)
	}

	carried := make(map[string]bool, len(current.StatusItems))
	for _, item := range current.StatusItems {
		carried[item] = true
	}
	for _, item := range previous.StatusItems {
		if !carried[item] {
			found.record("status item %s", item)
		}
	}
}

// compareKeys walks two key maps to the same depth the tables record, so a key removed from a
// nested dictionary is caught as well as a top-level one.
func compareKeys(found *removals, owner string, previous, current map[string]*schema) {
	for name, was := range previous {
		is, kept := current[name]
		if !kept {
			found.record("%s key %s", owner, name)
			continue
		}
		if was == nil || is == nil {
			continue
		}
		compareKeys(found, owner+"."+name, was.Keys, is.Keys)
		if was.Item != nil && is.Item != nil {
			compareKeys(found, owner+"."+name+"[]", was.Item.Keys, is.Item.Keys)
		}
	}
}

// record adds one finding.
func (r *removals) record(format string, args ...any) {
	r.Findings = append(r.Findings, fmt.Sprintf(format, args...))
}

// report renders the findings for stderr, naming at most maxReportedRemovals of them.
func (r *removals) report() string {
	shown := r.Findings
	var elided int
	if len(shown) > maxReportedRemovals {
		elided = len(shown) - maxReportedRemovals
		shown = shown[:maxReportedRemovals]
	}

	var builder strings.Builder
	for _, finding := range shown {
		fmt.Fprintf(&builder, "  - %s\n", finding)
	}
	if elided > 0 {
		fmt.Fprintf(&builder, "  - ... and %d more\n", elided)
	}
	return builder.String()
}
