//go:build skipkeycanary

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// IssueLabel marks every issue the canary files.
const IssueLabel = "skip-key-drift"

// Issue is an existing issue the canary may have filed.
type Issue struct {
	Number int
	Title  string
	Open   bool
}

// Tracker is the slice of GitHub issues the canary needs.
type Tracker interface {
	List(ctx context.Context) ([]Issue, error)
	Create(ctx context.Context, title, body string) error
	Edit(ctx context.Context, number int, title, body string) error
	Reopen(ctx context.Context, number int) error
	Comment(ctx context.Context, number int, body string) error
}

// Action says what Reconcile did for one direction.
type Action string

// The outcomes of reconciling one direction.
const (
	ActionNone     Action = "none"
	ActionCreated  Action = "created"
	ActionUpdated  Action = "updated"
	ActionSkipped  Action = "skipped"
	ActionReopened Action = "reopened"
)

// Decide chooses what to do about one direction given the issues already filed. An open issue with
// the exact title already tracks the drift, so nothing is filed. A closed one with the exact title
// means a fixed drift has come back, or a person closed the issue without changing the provider's
// table; either way the drift is still there, so it is reopened with a comment. The comment is
// what stops the reopen from reading as noise: the fix is to change the table, not to close the
// issue. An open issue for the same direction with a different key set is edited in place.
// Anything else files a new issue.
func Decide(dir Direction, title string, existing []Issue) (Action, int) {
	prefix := TitlePrefix(dir)
	update, closed := 0, 0
	for _, is := range existing {
		switch {
		case is.Title == title && is.Open:
			return ActionSkipped, is.Number
		case is.Title == title && closed == 0:
			closed = is.Number
		case is.Open && strings.HasPrefix(is.Title, prefix) && update == 0:
			update = is.Number
		}
	}
	switch {
	case closed != 0:
		return ActionReopened, closed
	case update != 0:
		return ActionUpdated, update
	}
	return ActionCreated, 0
}

// Reconcile files, updates or skips one issue per direction that has drift.
func Reconcile(ctx context.Context, tr Tracker, drift map[Direction][]Drift, apple map[string]string, logf func(string, ...any)) error {
	existing, err := tr.List(ctx)
	if err != nil {
		return fmt.Errorf("list %s issues: %w", IssueLabel, err)
	}
	for _, dir := range []Direction{New, Removed} {
		list := drift[dir]
		if len(list) == 0 {
			continue
		}
		title := IssueTitle(dir, list)
		body := IssueBody(dir, list, apple)
		action, number := Decide(dir, title, existing)
		switch action {
		case ActionSkipped:
			logf("issue #%d already tracks this %s drift", number, dir)
		case ActionReopened:
			logf("reopening issue #%d: the %s drift is back", number, dir)
			if err := tr.Reopen(ctx, number); err != nil {
				return fmt.Errorf("reopen issue #%d: %w", number, err)
			}
			if err := tr.Comment(ctx, number, ReopenComment(dir)); err != nil {
				return fmt.Errorf("comment on issue #%d: %w", number, err)
			}
		case ActionUpdated:
			logf("updating issue #%d for %s drift", number, dir)
			if err := tr.Edit(ctx, number, title, body); err != nil {
				return fmt.Errorf("update issue #%d: %w", number, err)
			}
		default:
			logf("opening an issue for %s drift", dir)
			if err := tr.Create(ctx, title, body); err != nil {
				return fmt.Errorf("open issue: %w", err)
			}
		}
	}
	return nil
}

// ReopenComment is the comment left on an issue the canary reopens.
func ReopenComment(dir Direction) string {
	var what string
	switch dir {
	case New:
		what = "Jamf Pro still returns the keys listed above and the provider still does not manage them."
	default:
		what = "The provider still manages the keys listed above and Jamf Pro still does not return them."
	}
	return "The skip-key canary found this drift again, so it reopened the issue. " + what +
		" Update the provider's skip-item table to close it for good; closing the issue alone makes the next daily run reopen it."
}

// ghTracker drives the gh CLI. The runner field exists so a test can record the arguments instead
// of starting a process; the zero value runs gh.
type ghTracker struct {
	runner func(ctx context.Context, args ...string) ([]byte, error)
}

func (g ghTracker) run(ctx context.Context, args ...string) ([]byte, error) {
	if g.runner != nil {
		return g.runner(ctx, args...)
	}
	return execGH(ctx, args...)
}

func execGH(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.Output()
	if err != nil {
		var detail string
		if ee, ok := err.(*exec.ExitError); ok {
			detail = strings.TrimSpace(string(ee.Stderr))
		}
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:2], " "), err, detail)
	}
	return out, nil
}

func (g ghTracker) List(ctx context.Context) ([]Issue, error) {
	if _, err := g.run(ctx, "label", "create", IssueLabel, "--description", "A PreStage skip key the provider and Jamf Pro disagree about", "--color", "FBCA04", "--force"); err != nil {
		return nil, err
	}
	out, err := g.run(ctx, "issue", "list", "--label", IssueLabel, "--state", "all", "--limit", "200", "--json", "number,title,state")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	issues := make([]Issue, 0, len(raw))
	for _, r := range raw {
		issues = append(issues, Issue{Number: r.Number, Title: r.Title, Open: strings.EqualFold(r.State, "OPEN")})
	}
	return issues, nil
}

func (g ghTracker) Create(ctx context.Context, title, body string) error {
	_, err := g.run(ctx, "issue", "create", "--label", IssueLabel, "--title", title, "--body", body)
	return err
}

func (g ghTracker) Edit(ctx context.Context, number int, title, body string) error {
	_, err := g.run(ctx, "issue", "edit", strconv.Itoa(number), "--title", title, "--body", body)
	return err
}

func (g ghTracker) Reopen(ctx context.Context, number int) error {
	_, err := g.run(ctx, "issue", "reopen", strconv.Itoa(number))
	return err
}

func (g ghTracker) Comment(ctx context.Context, number int, body string) error {
	_, err := g.run(ctx, "issue", "comment", strconv.Itoa(number), "--body", body)
	return err
}
