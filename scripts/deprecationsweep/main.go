//go:build deprecationsweep

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Command deprecationsweep reports the schema deprecations whose removal date has arrived.
//
// A deprecated attribute carries a greppable marker comment naming the earliest day it may be
// removed (STYLE_GUIDE.md, "Jamf Pro Terraform schema deprecation"). Nothing else in the
// repository reads that date, so without this tool a deprecation can sit past its window
// indefinitely. The weekly workflow in .github/workflows/deprecation-sweep.yml runs it and opens
// one tracking issue per expired marker.
//
// Parsing lives in internal/common/deprecations so that internal/conformance can assert, on every
// build, that each marker is well formed. This command adds only the date comparison, which is
// kept out of the test suite on purpose: a test that fails when a date passes breaks unrelated
// work on a day nobody chose.
//
// Output on stdout is the report and nothing else. The default format is a JSON array, empty as
// `[]`, which the workflow reads with jq; -format markdown prints a table for a person. A marker
// that does not parse is a failure: the offending lines go to stderr and the exit status is 1, so
// a malformed marker cannot hide an expiry.
//
// Usage:
//
//	go run -tags deprecationsweep ./scripts/deprecationsweep
//	go run -tags deprecationsweep ./scripts/deprecationsweep -today 2027-01-01 -format markdown
//
// The build tag keeps the tool out of `go build ./...` while still letting
// `go test -tags deprecationsweep ./scripts/deprecationsweep` exercise it, as for scripts/acclanes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/deprecations"
)

// report is one expired marker as the workflow consumes it. Title is built here so the workflow
// and any person reading the report agree on the string used to find an existing issue.
type report struct {
	deprecations.Marker
	Title string `json:"title"`
}

func main() {
	root := flag.String("root", "internal", "directory to scan for markers")
	today := flag.String("today", "", "date to compare against, YYYY-MM-DD (default: today, UTC)")
	format := flag.String("format", "json", "output format: json or markdown")
	flag.Parse()

	if err := run(os.Stdout, os.Stderr, *root, *today, *format, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, "deprecationsweep:", err)
		os.Exit(1)
	}
}

func run(stdout, stderr io.Writer, root, today, format string, now time.Time) error {
	day := now.UTC()
	if today != "" {
		parsed, err := time.Parse(deprecations.DateLayout, today)
		if err != nil {
			return fmt.Errorf("-today %q is not YYYY-MM-DD", today)
		}
		day = parsed
	}
	if format != "json" && format != "markdown" {
		return fmt.Errorf("-format %q is not json or markdown", format)
	}

	markers, problems, err := deprecations.Scan(root)
	if err != nil {
		return err
	}
	for _, p := range problems {
		_, _ = fmt.Fprintln(stderr, p)
	}

	expired := deprecations.Expired(markers, day)
	reports := make([]report, 0, len(expired))
	for _, m := range expired {
		reports = append(reports, report{Marker: m, Title: issueTitle(m)})
	}

	if format == "markdown" {
		writeMarkdown(stdout, reports)
	} else {
		out, err := json.MarshalIndent(reports, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(stdout, string(out))
	}

	if len(problems) > 0 {
		return fmt.Errorf("%d malformed marker(s)", len(problems))
	}
	return nil
}

// issueTitle names the tracking issue for a marker. It carries the file rather than the line,
// because lines move and a title that moved with them would open a duplicate.
func issueTitle(m deprecations.Marker) string {
	subject := "remove the attributes deprecated in favour of " + m.ReplacedBy
	if m.ReplacedBy == "none" {
		subject = "remove the deprecated attributes that have no replacement"
	}
	return fmt.Sprintf("removal(%s): %s", trimInternal(m.File), subject)
}

func trimInternal(file string) string {
	return strings.TrimSuffix(strings.TrimPrefix(path.Clean(file), "internal/"), "/")
}

func writeMarkdown(w io.Writer, reports []report) {
	if len(reports) == 0 {
		_, _ = fmt.Fprintln(w, "No deprecation has reached its removal date.")
		return
	}
	_, _ = fmt.Fprintln(w, "| Location | Remove after | Replaced by | Reason |")
	_, _ = fmt.Fprintln(w, "|---|---|---|---|")
	for _, r := range reports {
		_, _ = fmt.Fprintf(w, "| `%s:%d` | %s | `%s` | %s |\n", r.File, r.Line, r.RemoveAfter, r.ReplacedBy, r.Reason)
	}
}
