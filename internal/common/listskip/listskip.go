// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package listskip reports the items a list resource left out of its results
// because they could not be hydrated, so that one bad object never fails a
// whole `terraform query` and is never dropped silently either.
//
// A list resource hydrating each item for config generation can fail on one
// item alone: a per-item read refused, or a state builder whose diagnostics
// reject that object's data (issue #568: one policy with a repeated script
// failed every policy in the tenant). Assigning
// list.ListResultsStreamDiagnostics in that case replaces the whole stream,
// and a per-item `continue` with only a log line hides the object from the
// operator. The shape that does neither is the one patch_policy settled on:
// skip the item, record it, and push one trailing diagnostics-only result
// naming every skipped item.
//
// That trailing result must be a list.ListResult literal carrying nothing but
// Diagnostics. terraform-plugin-framework v1.19.0 passes such a result through
// untouched (internal/fwserver/server_listresource.go, around lines 180-205),
// while one built with ListRequest.NewListResult carries a non-nil, null
// Identity and is turned into an error, losing the warning.
package listskip

import (
	"fmt"
	"iter"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
)

// Item is one object a list resource left out of its results.
type Item struct {
	// ID is the object's identifier as the list enumerated it.
	ID string
	// Name is the object's display name, which is what an operator searches
	// the admin UI for.
	Name string
	// Diags is why it was skipped: the diagnostics hydration produced.
	Diags diag.Diagnostics
}

// Recorder accumulates skipped items during one List call. The zero value is
// ready to use.
type Recorder struct {
	items []Item
}

// Add records one skipped item.
func (r *Recorder) Add(item Item) {
	r.items = append(r.items, item)
}

// Len reports how many items were skipped.
func (r *Recorder) Len() int {
	return len(r.items)
}

// Result returns the trailing diagnostics-only result naming every skipped
// item, and false when nothing was skipped. kind is the plural noun the
// summary uses ("Jamf Pro policies"); hint, when not empty, closes the detail
// with what the operator can do about it.
func (r *Recorder) Result(kind, hint string) (list.ListResult, bool) {
	if len(r.items) == 0 {
		return list.ListResult{}, false
	}
	return list.ListResult{
		Diagnostics: diag.Diagnostics{
			diag.NewWarningDiagnostic(
				fmt.Sprintf("Some %s were left out of the results", kind),
				r.detail(kind, hint),
			),
		},
	}, true
}

// detail renders the operator-facing body of the trailing warning.
func (r *Recorder) detail(kind, hint string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d of the %s listed could not be read into configuration, so they are not in the results:\n", len(r.items), kind)
	for _, it := range r.items {
		fmt.Fprintf(&b, "\n  - %s (id %s)\n", it.Name, it.ID)
		for _, d := range it.Diags {
			if d.Severity() != diag.SeverityError {
				continue
			}
			line := d.Summary()
			if dp, ok := d.(diag.DiagnosticWithPath); ok && !dp.Path().Equal(path.Empty()) {
				line += " at " + dp.Path().String()
			}
			if detail := strings.TrimSpace(d.Detail()); detail != "" {
				line += ": " + strings.Join(strings.Fields(detail), " ")
			}
			fmt.Fprintf(&b, "      %s\n", line)
		}
	}
	if hint != "" {
		b.WriteString("\n")
		b.WriteString(hint)
	}
	return b.String()
}

// Stream returns the results followed by the trailing warning, if anything
// was skipped. With no results and nothing skipped it returns
// list.NoListResults, so a caller never needs its own empty-result branch and
// can never return the empty stream while a skip still needs reporting.
func Stream(results []list.ListResult, r *Recorder, kind, hint string) iter.Seq[list.ListResult] {
	trailing, skipped := r.Result(kind, hint)
	if len(results) == 0 && !skipped {
		return list.NoListResults
	}
	return func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
		if skipped {
			push(trailing)
		}
	}
}
