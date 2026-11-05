//go:build skipkeycanary

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Direction says which way a key set has drifted from the provider's table.
type Direction string

// The two ways a key set can drift.
const (
	New     Direction = "new"
	Removed Direction = "removed"
)

// Drift is one key on one platform that the wire and the provider disagree about.
type Drift struct {
	Kind Kind
	Key  string
}

// Diff compares the keys the API returned with the keys the provider manages, and returns what is
// only on the wire and what is only in the table. Both results are sorted.
func Diff(observed, known []string) (onlyWire, onlyTable []string) {
	have := make(map[string]bool, len(known))
	for _, k := range known {
		have[k] = true
	}
	seen := make(map[string]bool, len(observed))
	for _, k := range observed {
		seen[k] = true
		if !have[k] {
			onlyWire = append(onlyWire, k)
		}
	}
	for _, k := range known {
		if !seen[k] {
			onlyTable = append(onlyTable, k)
		}
	}
	sort.Strings(onlyWire)
	sort.Strings(onlyTable)
	return onlyWire, onlyTable
}

// Collect gathers drift for every platform into the two directions.
func Collect(observed, known map[Kind][]string) map[Direction][]Drift {
	out := map[Direction][]Drift{}
	for _, kind := range Kinds {
		onlyWire, onlyTable := Diff(observed[kind], known[kind])
		for _, k := range onlyWire {
			out[New] = append(out[New], Drift{Kind: kind, Key: k})
		}
		for _, k := range onlyTable {
			out[Removed] = append(out[Removed], Drift{Kind: kind, Key: k})
		}
	}
	return out
}

// Fingerprint is a short stable hash of a drift list, so the same set of keys always produces the
// same issue title and a changed set produces a different one.
func Fingerprint(drift []Drift) string {
	lines := make([]string, 0, len(drift))
	for _, d := range drift {
		lines = append(lines, string(d.Kind)+":"+d.Key)
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])[:8]
}

// TitlePrefix is the part of an issue title that identifies its direction.
func TitlePrefix(dir Direction) string {
	switch dir {
	case New:
		return "PreStage skip keys on the wire but not in the provider"
	default:
		return "PreStage skip keys in the provider but not on the wire"
	}
}

// IssueTitle is the stable title for one direction and one set of keys.
func IssueTitle(dir Direction, drift []Drift) string {
	return fmt.Sprintf("%s (%s)", TitlePrefix(dir), Fingerprint(drift))
}

// ParseAppleTitles reads the key and title pairs out of Apple's skipkeys.yaml. Keys are lower
// cased so a match ignores case. The file is read line by line: a top-level payload key is
// indented two spaces and its title sits directly beneath it at the same depth.
func ParseAppleTitles(yaml string) map[string]string {
	titles := map[string]string{}
	current := ""
	for line := range strings.SplitSeq(yaml, "\n") {
		switch {
		case strings.HasPrefix(line, "- key: "):
			current = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "- key: ")))
		case current != "" && strings.HasPrefix(line, "  title: "):
			titles[current] = strings.TrimSpace(strings.TrimPrefix(line, "  title: "))
			current = ""
		}
	}
	return titles
}

// IssueBody renders the issue text. Apple's title is only a hint for naming; it is shown when the
// key matches and left out when it does not.
func IssueBody(dir Direction, drift []Drift, apple map[string]string) string {
	var b strings.Builder
	switch dir {
	case New:
		b.WriteString("The Jamf Pro API returns `skipSetupItems` keys that neither `jamfplatform_pro_computer_prestage_enrollment` nor `jamfplatform_pro_mobile_device_prestage_enrollment` manages.\n\n")
		b.WriteString("Each key needs an attribute named after its label in the Jamf Pro admin UI. The API returns the key without the label, so someone has to supply it.\n\n")
		b.WriteString("**Attach a screenshot of the PreStage *Setup Assistant Options* list from the Jamf Pro admin UI**, computer and mobile device, so each key below can be matched to its UI label.\n\n")
	default:
		b.WriteString("The provider manages `skipSetupItems` keys that Jamf Pro no longer returns.\n\n")
		b.WriteString("Jamf probably removed these panes. Check the PreStage *Setup Assistant Options* list in the Jamf Pro admin UI and deprecate the attribute if it is gone.\n\n")
	}
	b.WriteString("| Platform | Wire key | Apple's name for the pane |\n| --- | --- | --- |\n")
	ordered := append([]Drift(nil), drift...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind < ordered[j].Kind
		}
		return ordered[i].Key < ordered[j].Key
	})
	for _, d := range ordered {
		title := apple[strings.ToLower(d.Key)]
		if title == "" {
			title = "no match in Apple's list"
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s |\n", d.Kind, d.Key, title)
	}
	b.WriteString("\nApple's list (`other/skipkeys.yaml` in apple/device-management) only hints at names. Jamf uses keys Apple does not list, and Apple lists keys Jamf does not use, so a missing match means nothing.\n\n")
	b.WriteString("Filed by the skip-key canary (`.github/workflows/acceptance.yml`, `pro` lane).\n")
	return b.String()
}
