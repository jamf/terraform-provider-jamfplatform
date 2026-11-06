// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package deprecations

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		candidate  bool
		wantDetail bool
		want       Marker
	}{
		{"platform with reason", "// PLATFORM-DEPRECATED remove-after=2026-10-22 replaced-by=component_blocks — superseded", true, false,
			Marker{Kind: "PLATFORM", RemoveAfter: "2026-10-22", ReplacedBy: "component_blocks", Reason: "superseded"}},
		{"pro without reason", "\t// PRO-DEPRECATED remove-after=2027-02-01 replaced-by=script_assignments", true, false,
			Marker{Kind: "PRO", RemoveAfter: "2027-02-01", ReplacedBy: "script_assignments"}},
		{"ascii dash reason", "// PRO-DEPRECATED remove-after=2027-02-01 replaced-by=none - no replacement", true, false,
			Marker{Kind: "PRO", RemoveAfter: "2027-02-01", ReplacedBy: "none", Reason: "no replacement"}},
		{"not a marker", "// an ordinary comment", false, false, Marker{}},
		{"prose mentioning one", "// the PRO-DEPRECATED marker is documented elsewhere", false, false, Marker{}},
		{"missing replaced-by", "// PRO-DEPRECATED remove-after=2027-02-01", true, true, Marker{}},
		{"missing date", "// PRO-DEPRECATED replaced-by=x", true, true, Marker{}},
		{"impossible date", "// PRO-DEPRECATED remove-after=2027-02-30 replaced-by=x", true, true, Marker{}},
		{"two-digit year", "// PRO-DEPRECATED remove-after=27-02-01 replaced-by=x", true, true, Marker{}},
		{"unknown kind", "// OTHER-DEPRECATED remove-after=2027-02-01 replaced-by=x", false, false, Marker{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, detail, ok := Parse(tc.line)
			if ok != tc.candidate {
				t.Fatalf("candidate = %v, want %v", ok, tc.candidate)
			}
			if (detail != "") != tc.wantDetail {
				t.Fatalf("detail = %q, want problem = %v", detail, tc.wantDetail)
			}
			if got != tc.want {
				t.Fatalf("marker = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "internal")
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScan(t *testing.T) {
	root := writeTree(t, map[string]string{
		"b/two.go":                  "package b\n\n// PRO-DEPRECATED remove-after=2027-03-01 replaced-by=y\nconst x = 1\n",
		"a/one.go":                  "package a\n// PLATFORM-DEPRECATED remove-after=2026-10-22 replaced-by=x — why\n// PRO-DEPRECATED remove-after=bad replaced-by=x\n",
		"a/one_test.go":             "package a\n// PRO-DEPRECATED garbage\n",
		"a/testdata/fixture.go":     "package fixture\n// PRO-DEPRECATED garbage\n",
		"common/deprecations/me.go": "package deprecations\n// PRO-DEPRECATED garbage\n",
		"a/notes.md":                "// PRO-DEPRECATED garbage\n",
	})
	markers, problems, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 2 || markers[0].File != "internal/a/one.go" || markers[0].Line != 2 || markers[1].File != "internal/b/two.go" || markers[1].Line != 3 {
		t.Fatalf("markers = %+v", markers)
	}
	if len(problems) != 1 || problems[0].File != "internal/a/one.go" || problems[0].Line != 3 {
		t.Fatalf("problems = %+v", problems)
	}
}

func TestExpired(t *testing.T) {
	markers := []Marker{
		{RemoveAfter: "2026-10-22"},
		{RemoveAfter: "2026-11-05"},
		{RemoveAfter: "2026-11-06"},
	}
	got := Expired(markers, time.Date(2026, 11, 5, 23, 0, 0, 0, time.UTC))
	if len(got) != 2 || got[1].RemoveAfter != "2026-11-05" {
		t.Fatalf("expired = %+v, want the first two (the day itself counts)", got)
	}
	if got := Expired(markers, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); len(got) != 0 {
		t.Fatalf("expired = %+v, want none", got)
	}
}
