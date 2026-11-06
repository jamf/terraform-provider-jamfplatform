//go:build deprecationsweep

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tree(t *testing.T, files map[string]string) string {
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

var fixture = map[string]string{
	"resources/blueprints/blueprint/resource.go": "package blueprint\n\n// PLATFORM-DEPRECATED remove-after=2026-10-22 replaced-by=component_blocks — flat attributes\nconst c = 1\n",
	"resources/pro/policy/resource.go":           "package policy\n\n// PRO-DEPRECATED remove-after=2027-02-10 replaced-by=none — read-only mirror\nconst c = 1\n",
}

func TestRunReportsOnlyExpiredMarkers(t *testing.T) {
	root := tree(t, fixture)
	var out, errOut bytes.Buffer
	if err := run(&out, &errOut, root, "2026-11-05", "json", time.Time{}); err != nil {
		t.Fatalf("run: %v (%s)", err, errOut.String())
	}
	var got []report
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].ReplacedBy != "component_blocks" || got[0].Line != 3 {
		t.Fatalf("report = %+v", got)
	}
	want := "removal(resources/blueprints/blueprint/resource.go): remove the attributes deprecated in favour of component_blocks"
	if got[0].Title != want {
		t.Fatalf("title = %q, want %q", got[0].Title, want)
	}
}

func TestRunLaterDateReportsBoth(t *testing.T) {
	root := tree(t, fixture)
	var out bytes.Buffer
	if err := run(&out, &bytes.Buffer{}, root, "2027-02-10", "json", time.Time{}); err != nil {
		t.Fatal(err)
	}
	var got []report
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !strings.Contains(got[1].Title, "no replacement") {
		t.Fatalf("report = %+v", got)
	}
}

func TestRunEmptyIsAnEmptyArray(t *testing.T) {
	root := tree(t, fixture)
	var out bytes.Buffer
	if err := run(&out, &bytes.Buffer{}, root, "2026-01-01", "json", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("output = %q, want []", out.String())
	}
}

func TestRunMalformedMarkerFails(t *testing.T) {
	root := tree(t, map[string]string{"x/x.go": "package x\n// PRO-DEPRECATED remove-after=soon replaced-by=y\n"})
	var out, errOut bytes.Buffer
	if err := run(&out, &errOut, root, "2030-01-01", "json", time.Time{}); err == nil {
		t.Fatal("want an error for a malformed marker")
	}
	if !strings.Contains(errOut.String(), "x/x.go:2") {
		t.Fatalf("stderr = %q, want the offending line named", errOut.String())
	}
}

func TestRunRejectsBadFlags(t *testing.T) {
	root := tree(t, fixture)
	if err := run(&bytes.Buffer{}, &bytes.Buffer{}, root, "05/11/2026", "json", time.Time{}); err == nil {
		t.Fatal("want an error for a bad -today")
	}
	if err := run(&bytes.Buffer{}, &bytes.Buffer{}, root, "", "xml", time.Time{}); err == nil {
		t.Fatal("want an error for a bad -format")
	}
}

func TestMarkdown(t *testing.T) {
	root := tree(t, fixture)
	var out bytes.Buffer
	if err := run(&out, &bytes.Buffer{}, root, "2026-11-05", "markdown", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "| `internal/resources/blueprints/blueprint/resource.go:3` | 2026-10-22 | `component_blocks` |") {
		t.Fatalf("markdown = %q", out.String())
	}
}
