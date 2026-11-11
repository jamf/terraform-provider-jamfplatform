// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package listskip

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
)

func collect(seq func(func(list.ListResult) bool)) []list.ListResult {
	var out []list.ListResult
	for r := range seq {
		out = append(out, r)
	}
	return out
}

func TestRecorder_NothingSkippedHasNoResult(t *testing.T) {
	var r Recorder
	if _, ok := r.Result("things", ""); ok {
		t.Error("an empty recorder reported a result")
	}
	if got := collect(Stream(nil, &r, "things", "")); len(got) != 0 {
		t.Errorf("empty stream pushed %d results", len(got))
	}
}

func TestRecorder_ResultNamesEveryItemAndError(t *testing.T) {
	var r Recorder
	r.Add(Item{ID: "7", Name: "Seven", Diags: diag.Diagnostics{
		diag.NewAttributeErrorDiagnostic(path.Root("script_assignments"), "Duplicate Set Element", "This attribute contains\nduplicate values."),
		diag.NewWarningDiagnostic("ignored warning", "not an error"),
	}})
	r.Add(Item{ID: "9", Name: "Nine", Diags: diag.Diagnostics{diag.NewErrorDiagnostic("Unable to read", "HTTP 500")}})

	res, ok := r.Result("Jamf Pro policies", "Try again.")
	if !ok {
		t.Fatal("no result")
	}
	if res.Identity != nil || res.Resource != nil || res.DisplayName != "" {
		t.Error("the trailing result must carry diagnostics only")
	}
	if len(res.Diagnostics) != 1 || res.Diagnostics.HasError() {
		t.Fatalf("diagnostics = %v, want one warning", res.Diagnostics)
	}
	d := res.Diagnostics[0]
	if d.Summary() != "Some Jamf Pro policies were left out of the results" {
		t.Errorf("summary = %q", d.Summary())
	}
	for _, want := range []string{
		"2 of the Jamf Pro policies",
		"Seven (id 7)",
		"Duplicate Set Element at script_assignments: This attribute contains duplicate values.",
		"Nine (id 9)",
		"Unable to read: HTTP 500",
		"Try again.",
	} {
		if !strings.Contains(d.Detail(), want) {
			t.Errorf("detail missing %q:\n%s", want, d.Detail())
		}
	}
	if strings.Contains(d.Detail(), "ignored warning") {
		t.Errorf("detail repeats a non-error diagnostic:\n%s", d.Detail())
	}
}

func TestStream_AppendsTheWarningAfterResults(t *testing.T) {
	var r Recorder
	r.Add(Item{ID: "1", Name: "One", Diags: diag.Diagnostics{diag.NewErrorDiagnostic("x", "")}})
	results := []list.ListResult{{DisplayName: "a"}, {DisplayName: "b"}}

	got := collect(Stream(results, &r, "things", ""))
	if len(got) != 3 || got[0].DisplayName != "a" || got[1].DisplayName != "b" || len(got[2].Diagnostics) != 1 {
		t.Fatalf("got %v", got)
	}

	if got := collect(Stream(nil, &r, "things", "")); len(got) != 1 {
		t.Errorf("all-skipped stream pushed %d results, want the warning alone", len(got))
	}

	var stopped []list.ListResult
	for res := range Stream(results, &r, "things", "") {
		stopped = append(stopped, res)
		break
	}
	if len(stopped) != 1 {
		t.Errorf("stream ignored a stop request")
	}
}
