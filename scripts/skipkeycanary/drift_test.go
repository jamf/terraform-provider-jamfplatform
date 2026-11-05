//go:build skipkeycanary

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestDiffSplitsWireOnlyFromTableOnly(t *testing.T) {
	onlyWire, onlyTable := Diff([]string{"A", "C", "B"}, []string{"B", "D", "A"})
	if !reflect.DeepEqual(onlyWire, []string{"C"}) || !reflect.DeepEqual(onlyTable, []string{"D"}) {
		t.Fatalf("onlyWire=%v onlyTable=%v", onlyWire, onlyTable)
	}
}

func TestDiffNoDrift(t *testing.T) {
	onlyWire, onlyTable := Diff([]string{"A", "B"}, []string{"B", "A"})
	if len(onlyWire)+len(onlyTable) != 0 {
		t.Fatalf("onlyWire=%v onlyTable=%v", onlyWire, onlyTable)
	}
}

func TestCollectKeysDriftByDirectionAndPlatform(t *testing.T) {
	got := Collect(
		map[Kind][]string{Computer: {"A", "Z"}, Mobile: {"A"}},
		map[Kind][]string{Computer: {"A"}, Mobile: {"A", "Gone"}},
	)
	if !reflect.DeepEqual(got[New], []Drift{{Computer, "Z"}}) {
		t.Fatalf("new = %v", got[New])
	}
	if !reflect.DeepEqual(got[Removed], []Drift{{Mobile, "Gone"}}) {
		t.Fatalf("removed = %v", got[Removed])
	}
}

func TestFingerprintIgnoresOrderAndTracksContent(t *testing.T) {
	a := []Drift{{Computer, "X"}, {Mobile, "Y"}}
	b := []Drift{{Mobile, "Y"}, {Computer, "X"}}
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("fingerprint must not depend on order")
	}
	if Fingerprint(a) == Fingerprint([]Drift{{Computer, "X"}}) {
		t.Fatal("fingerprint must change with the key set")
	}
	if len(Fingerprint(a)) != 8 {
		t.Fatalf("fingerprint = %q", Fingerprint(a))
	}
}

func TestIssueTitleIsStableAndDirectional(t *testing.T) {
	d := []Drift{{Computer, "X"}}
	same := []Drift{{Computer, "X"}}
	if IssueTitle(New, d) != IssueTitle(New, same) {
		t.Fatal("title must be stable")
	}
	if IssueTitle(New, d) == IssueTitle(Removed, d) {
		t.Fatal("directions must not share a title")
	}
	if !strings.HasPrefix(IssueTitle(New, d), TitlePrefix(New)) {
		t.Fatalf("title = %q", IssueTitle(New, d))
	}
}

const appleSample = `title: SkipKeys
payload:
  payloadtype: SkipKeys
payloadkeys:
- key: Accessibility
  title: Skip Accessibility pane
  supportOS:
    iOS:
      introduced: n/a
  content: |-
    title: not this one
- key: LiquidGlass
  title: Skip Liquid Glass pane
  type: <string>
`

func TestParseAppleTitlesReadsKeyAndTitlePairs(t *testing.T) {
	got := ParseAppleTitles(appleSample)
	want := map[string]string{"accessibility": "Skip Accessibility pane", "liquidglass": "Skip Liquid Glass pane"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestIssueBodyShowsAppleTitleOnlyWhenKeyMatches(t *testing.T) {
	apple := ParseAppleTitles(appleSample)
	body := IssueBody(New, []Drift{{Mobile, "liquidglass"}, {Computer, "Brand New"}}, apple)
	for _, want := range []string{
		"| mobile | `liquidglass` | Skip Liquid Glass pane |",
		"| computer | `Brand New` | no match in Apple's list |",
		"screenshot",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Index(body, "| computer |") > strings.Index(body, "| mobile |") {
		t.Error("rows must be ordered by platform")
	}
}

func TestIssueBodyForRemovedKeysDoesNotAskForAScreenshot(t *testing.T) {
	body := IssueBody(Removed, []Drift{{Mobile, "Gone"}}, nil)
	if strings.Contains(body, "screenshot") {
		t.Fatalf("body = %s", body)
	}
	if !strings.Contains(body, "`Gone`") {
		t.Fatalf("body = %s", body)
	}
}
