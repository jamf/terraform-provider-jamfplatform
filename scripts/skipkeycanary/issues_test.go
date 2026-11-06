//go:build skipkeycanary

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

type fakeTracker struct {
	issues    []Issue
	created   []string
	edited    map[int]string
	reopened  []int
	commented map[int]string
}

func (f *fakeTracker) Reopen(_ context.Context, number int) error {
	f.reopened = append(f.reopened, number)
	return nil
}

func (f *fakeTracker) Comment(_ context.Context, number int, body string) error {
	if f.commented == nil {
		f.commented = map[int]string{}
	}
	f.commented[number] = body
	return nil
}

func (f *fakeTracker) List(context.Context) ([]Issue, error) { return f.issues, nil }

func (f *fakeTracker) Create(_ context.Context, title, _ string) error {
	f.created = append(f.created, title)
	return nil
}

func (f *fakeTracker) Edit(_ context.Context, number int, title, _ string) error {
	if f.edited == nil {
		f.edited = map[int]string{}
	}
	f.edited[number] = title
	return nil
}

var drift = map[Direction][]Drift{New: {{Computer, "X"}}}

func noLog(string, ...any) {}

func TestDecide(t *testing.T) {
	title := IssueTitle(New, drift[New])
	other := TitlePrefix(New) + " (deadbeef)"
	cases := []struct {
		name     string
		existing []Issue
		action   Action
		number   int
	}{
		{"nothing filed", nil, ActionCreated, 0},
		{"same title open", []Issue{{Number: 4, Title: title, Open: true}}, ActionSkipped, 4},
		{"same title closed comes back", []Issue{{Number: 5, Title: title, Open: false}}, ActionReopened, 5},
		{"an open duplicate wins over a closed one", []Issue{{Number: 5, Title: title, Open: false}, {Number: 9, Title: title, Open: true}}, ActionSkipped, 9},
		{"a closed exact match wins over editing an open near match", []Issue{{Number: 6, Title: other, Open: true}, {Number: 5, Title: title, Open: false}}, ActionReopened, 5},
		{"open issue for a different key set is edited", []Issue{{Number: 6, Title: other, Open: true}}, ActionUpdated, 6},
		{"closed issue for a different key set is not reused", []Issue{{Number: 7, Title: other, Open: false}}, ActionCreated, 0},
		{"other direction is ignored", []Issue{{Number: 8, Title: TitlePrefix(Removed) + " (deadbeef)", Open: true}}, ActionCreated, 0},
	}
	for _, c := range cases {
		action, number := Decide(New, title, c.existing)
		if action != c.action || number != c.number {
			t.Errorf("%s: got %s #%d, want %s #%d", c.name, action, number, c.action, c.number)
		}
	}
}

func TestReconcileFilesOneIssuePerDirectionWithDrift(t *testing.T) {
	tr := &fakeTracker{}
	both := map[Direction][]Drift{New: {{Computer, "X"}}, Removed: {{Mobile, "Y"}}}
	if err := Reconcile(context.Background(), tr, both, nil, noLog); err != nil {
		t.Fatal(err)
	}
	if len(tr.created) != 2 {
		t.Fatalf("created = %v", tr.created)
	}
}

func TestReconcileIsIdempotent(t *testing.T) {
	tr := &fakeTracker{}
	if err := Reconcile(context.Background(), tr, drift, nil, noLog); err != nil {
		t.Fatal(err)
	}
	tr.issues = []Issue{{Number: 1, Title: tr.created[0], Open: true}}
	tr.created = nil
	if err := Reconcile(context.Background(), tr, drift, nil, noLog); err != nil {
		t.Fatal(err)
	}
	if len(tr.created) != 0 || len(tr.edited) != 0 {
		t.Fatalf("second pass changed issues: created=%v edited=%v", tr.created, tr.edited)
	}
}

func TestReconcileEditsAnOpenIssueWhenTheKeySetChanges(t *testing.T) {
	tr := &fakeTracker{issues: []Issue{{Number: 3, Title: TitlePrefix(New) + " (deadbeef)", Open: true}}}
	if err := Reconcile(context.Background(), tr, drift, nil, noLog); err != nil {
		t.Fatal(err)
	}
	if len(tr.created) != 0 || tr.edited[3] != IssueTitle(New, drift[New]) {
		t.Fatalf("created=%v edited=%v", tr.created, tr.edited)
	}
}

func TestReconcileReopensAClosedIssueAndSaysWhy(t *testing.T) {
	tr := &fakeTracker{issues: []Issue{{Number: 12, Title: IssueTitle(New, drift[New]), Open: false}}}
	if err := Reconcile(context.Background(), tr, drift, nil, noLog); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tr.reopened, []int{12}) || len(tr.created) != 0 || len(tr.edited) != 0 {
		t.Fatalf("reopened=%v created=%v edited=%v", tr.reopened, tr.created, tr.edited)
	}
	comment := tr.commented[12]
	for _, want := range []string{"found this drift again", "Jamf Pro still returns", "closing the issue alone"} {
		if !strings.Contains(comment, want) {
			t.Errorf("comment missing %q:\n%s", want, comment)
		}
	}
}

func TestReconcileDoesNotReopenAnOpenIssue(t *testing.T) {
	tr := &fakeTracker{issues: []Issue{{Number: 12, Title: IssueTitle(New, drift[New]), Open: true}}}
	if err := Reconcile(context.Background(), tr, drift, nil, noLog); err != nil {
		t.Fatal(err)
	}
	if len(tr.reopened) != 0 || len(tr.commented) != 0 {
		t.Fatalf("reopened=%v commented=%v", tr.reopened, tr.commented)
	}
}

func TestReopenCommentNamesTheDirection(t *testing.T) {
	if !strings.Contains(ReopenComment(Removed), "Jamf Pro still does not return them") {
		t.Fatalf("removed comment = %s", ReopenComment(Removed))
	}
}

func TestGHTrackerReopenAndCommentArguments(t *testing.T) {
	var calls [][]string
	g := ghTracker{runner: func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return nil, nil
	}}
	if err := g.Reopen(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if err := g.Comment(context.Background(), 7, "back again"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"issue", "reopen", "7"}, {"issue", "comment", "7", "--body", "back again"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("gh calls = %v, want %v", calls, want)
	}
}

func TestReconcileWithNoDriftTouchesNothing(t *testing.T) {
	tr := &fakeTracker{}
	if err := Reconcile(context.Background(), tr, map[Direction][]Drift{}, nil, noLog); err != nil {
		t.Fatal(err)
	}
	if len(tr.created) != 0 {
		t.Fatalf("created = %v", tr.created)
	}
}
