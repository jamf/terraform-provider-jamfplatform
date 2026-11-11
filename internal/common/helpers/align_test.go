// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"reflect"
	"strings"
	"testing"
)

type alignScript struct{ id, priority, param string }

func (s alignScript) String() string { return s.id + "/" + s.param + "@" + s.priority }

func alignScriptKey(s alignScript) string { return s.id + "\x00" + s.priority }

func alignScriptPriorKey(s alignScript) (string, bool) {
	if s.id == "" {
		return "", false
	}
	return alignScriptKey(s), true
}

func parseAlignScripts(t *testing.T, spec string) []alignScript {
	t.Helper()
	if spec == "" {
		return nil
	}
	var out []alignScript
	for f := range strings.FieldsSeq(spec) {
		idParam, prio, ok := strings.Cut(f, "@")
		if !ok {
			t.Fatalf("bad spec %q", f)
		}
		id, param, _ := strings.Cut(idParam, "/")
		if id == "?" {
			id = ""
		}
		out = append(out, alignScript{id: id, priority: prio, param: param})
	}
	return out
}

func renderAligned(server []alignScript, order []int) string {
	parts := make([]string, 0, len(order))
	for _, s := range order {
		parts = append(parts, server[s].String())
	}
	return strings.Join(parts, " ")
}

// TestAlignToPrior_Scripts drives the helper with the script orderings
// wire-probed for issue #568 (2026-11-11): Jamf Pro sorts a policy's scripts by
// id, then priority with After before Before, and keeps insertion order among
// entries sharing both.
func TestAlignToPrior_Scripts(t *testing.T) {
	tests := []struct {
		name         string
		server       string
		prior        string
		want         string
		wantPriorIdx []int
	}{
		{
			name:         "C A C A A stored grouped comes back in written order",
			server:       "2457/a1@After 2457/a2@After 2457/a3@After 2459/c1@After 2459/c2@After",
			prior:        "2459/c1@After 2457/a1@After 2459/c2@After 2457/a2@After 2457/a3@After",
			want:         "2459/c1@After 2457/a1@After 2459/c2@After 2457/a2@After 2457/a3@After",
			wantPriorIdx: []int{0, 1, 2, 3, 4},
		},
		{
			name:         "ids re-sorted ascending",
			server:       "2457/x@After 2458/x@After 2459/x@After",
			prior:        "2459/x@After 2457/x@After 2458/x@After",
			want:         "2459/x@After 2457/x@After 2458/x@After",
			wantPriorIdx: []int{0, 1, 2},
		},
		{
			name:         "same id keeps parameter order zzz aaa mmm",
			server:       "2458/zzz@After 2458/aaa@After 2458/mmm@After",
			prior:        "2458/zzz@After 2458/aaa@After 2458/mmm@After",
			want:         "2458/zzz@After 2458/aaa@After 2458/mmm@After",
			wantPriorIdx: []int{0, 1, 2},
		},
		{
			name:         "Before written ahead of After",
			server:       "2458/x@After 2458/x@Before",
			prior:        "2458/x@Before 2458/x@After",
			want:         "2458/x@Before 2458/x@After",
			wantPriorIdx: []int{0, 1},
		},
		{
			name:         "identical duplicates all kept",
			server:       "1/p@After 1/p@After 1/p@After",
			prior:        "1/p@After 1/p@After 1/p@After",
			want:         "1/p@After 1/p@After 1/p@After",
			wantPriorIdx: []int{0, 1, 2},
		},
		{
			name:         "server dropped an entry",
			server:       "1/a@After 3/c@After",
			prior:        "3/c@After 2/b@After 1/a@After",
			want:         "3/c@After 1/a@After",
			wantPriorIdx: []int{0, 2},
		},
		{
			name:         "server added an entry surfaces at the end",
			server:       "1/a@After 2/new@After 3/c@After",
			prior:        "3/c@After 1/a@After",
			want:         "3/c@After 1/a@After 2/new@After",
			wantPriorIdx: []int{0, 1, -1},
		},
		{
			name:         "server holds one more of a repeated entry",
			server:       "1/a@After 1/b@After",
			prior:        "1/a@After",
			want:         "1/a@After 1/b@After",
			wantPriorIdx: []int{0, -1},
		},
		{
			name:         "nil prior keeps server order",
			server:       "1/a@After 2/b@After",
			prior:        "",
			want:         "1/a@After 2/b@After",
			wantPriorIdx: []int{-1, -1},
		},
		{
			name:         "unknown prior key takes the unmatched server entry at its position",
			server:       "50/b@After 100/new@After",
			prior:        "?/new@After 50/b@After",
			want:         "100/new@After 50/b@After",
			wantPriorIdx: []int{0, 1},
		},
		{
			name:         "empty server",
			server:       "",
			prior:        "1/a@After",
			want:         "",
			wantPriorIdx: []int{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := parseAlignScripts(t, tt.server)
			prior := parseAlignScripts(t, tt.prior)
			order, priorIdx := AlignToPrior(server, prior, alignScriptKey, alignScriptPriorKey)
			if got := renderAligned(server, order); got != tt.want {
				t.Errorf("order:\n got  %s\n want %s", got, tt.want)
			}
			if !reflect.DeepEqual(priorIdx, tt.wantPriorIdx) {
				t.Errorf("priorIdx = %v, want %v", priorIdx, tt.wantPriorIdx)
			}
			for n, j := range priorIdx {
				if j < 0 {
					continue
				}
				if got, want := server[order[n]].param, prior[j].param; got != want {
					t.Errorf("position %d paired server param %q with prior param %q", n, got, want)
				}
			}
		})
	}
}

// TestAlignToPriorFunc_UnknownKeysUseFits pins the Create case where two new
// scripts' ids are not yet known: without a predicate they would pair in
// server order and swap parameters.
func TestAlignToPriorFunc_UnknownKeysUseFits(t *testing.T) {
	server := parseAlignScripts(t, "100/x@After 200/y@After")
	prior := parseAlignScripts(t, "?/y@After ?/x@After")
	fits := func(s, p alignScript) bool { return s.priority == p.priority && s.param == p.param }

	order, priorIdx := AlignToPriorFunc(server, prior, alignScriptKey, alignScriptPriorKey, fits)
	if got, want := renderAligned(server, order), "200/y@After 100/x@After"; got != want {
		t.Errorf("with fits: got %s, want %s", got, want)
	}
	if !reflect.DeepEqual(priorIdx, []int{0, 1}) {
		t.Errorf("priorIdx = %v", priorIdx)
	}

	order, _ = AlignToPrior(server, prior, alignScriptKey, alignScriptPriorKey)
	if got, want := renderAligned(server, order), "100/x@After 200/y@After"; got != want {
		t.Errorf("without fits: got %s, want %s", got, want)
	}
}

// TestAlignToPrior_PackagePermutations covers every permutation of three
// packages. Jamf Pro returns a policy's packages sorted by name whatever the
// written order (wire-probed 2026-11-11), so the server side is fixed and the
// prior side varies.
func TestAlignToPrior_PackagePermutations(t *testing.T) {
	type pkg struct{ id, name string }
	server := []pkg{{"3", "Aaa"}, {"1", "Mmm"}, {"2", "Zzz"}}
	key := func(p pkg) string { return p.id }
	priorKey := func(p pkg) (string, bool) { return p.id, true }
	for _, perm := range [][]string{
		{"1", "2", "3"}, {"1", "3", "2"}, {"2", "1", "3"},
		{"2", "3", "1"}, {"3", "1", "2"}, {"3", "2", "1"},
	} {
		prior := []pkg{{id: perm[0]}, {id: perm[1]}, {id: perm[2]}}
		order, priorIdx := AlignToPrior(server, prior, key, priorKey)
		got := make([]string, 0, len(order))
		for _, s := range order {
			got = append(got, server[s].id)
		}
		if !reflect.DeepEqual(got, perm) {
			t.Errorf("prior %v: got %v", perm, got)
		}
		if !reflect.DeepEqual(priorIdx, []int{0, 1, 2}) {
			t.Errorf("prior %v: priorIdx %v", perm, priorIdx)
		}
	}
}

// TestAlignToPrior_SameIDPackagesKeepInsertionOrder covers one package listed
// three times with different settings: the server keeps those in the order
// sent, so FIFO pairing hands each its own prior element.
func TestAlignToPrior_SameIDPackagesKeepInsertionOrder(t *testing.T) {
	type pkg struct{ id, action string }
	server := []pkg{{"2", "Install"}, {"1", "Install"}, {"1", "Cache"}, {"1", "Install"}}
	prior := []pkg{{"1", "Install"}, {"1", "Cache"}, {"2", "Install"}, {"1", "Install"}}
	order, priorIdx := AlignToPrior(server, prior, func(p pkg) string { return p.id }, func(p pkg) (string, bool) { return p.id, true })
	if !reflect.DeepEqual(order, []int{1, 2, 0, 3}) || !reflect.DeepEqual(priorIdx, []int{0, 1, 2, 3}) {
		t.Fatalf("order %v priorIdx %v", order, priorIdx)
	}
	for n, s := range order {
		if server[s].action != prior[priorIdx[n]].action {
			t.Errorf("position %d: server action %q paired with prior %q", n, server[s].action, prior[priorIdx[n]].action)
		}
	}
}
