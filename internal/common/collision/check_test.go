// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package collision

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSetKeys(t *testing.T) {
	set := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("a"), types.StringValue("b")})
	if got := SetKeys(set); len(got) != 2 {
		t.Fatalf("want 2 keys, got %v", got)
	}
	if got := SetKeys(types.SetNull(types.StringType)); got != nil {
		t.Fatalf("a null set has no keys, got %v", got)
	}
	got := SetKeys(types.SetUnknown(types.StringType))
	if len(got) != 1 || !got[0].IsUnknown() {
		t.Fatalf("an unknown set must yield one unknown key, got %v", got)
	}
	if _, known := knownValues(got); known {
		t.Fatal("an unknown set must make ReportPlan skip the rule")
	}
}

const testResourceType = "jamfplatform_pro_widget"

func widgetNamespace(entries []Entry, calls *atomic.Int32) Namespace {
	return Namespace{
		ID:        "pro.widgets",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: CollationFolded,
		Sources: []Source{{
			Noun:         "widget",
			ResourceType: testResourceType,
			List: func(context.Context) ([]Entry, error) {
				if calls != nil {
					calls.Add(1)
				}
				return entries, nil
			},
		}},
	}
}

func newChecker(mode Mode, includeUnenforced bool) *Checker {
	return New(Config{Mode: mode, IncludeUnenforced: includeUnenforced})
}

func request(ns Namespace, keys []string, prior []string, selfID string) Request {
	return Request{
		Namespace:    ns,
		ResourceType: testResourceType,
		Path:         path.Root("name"),
		Keys:         keys,
		PriorKeys:    prior,
		SelfID:       selfID,
	}
}

func TestNew_OffIsNil(t *testing.T) {
	if New(Config{Mode: ModeOff}) != nil {
		t.Fatal("ModeOff must build a nil checker")
	}
	if New(Config{}) != nil {
		t.Fatal("an unset mode must build a nil checker")
	}
}

func TestCheck_NilCheckerReportsNothing(t *testing.T) {
	var c *Checker
	ns := widgetNamespace([]Entry{{ID: "1", Key: "a"}}, nil)
	if d := c.Check(context.Background(), request(ns, []string{"a"}, nil, "")); len(d) != 0 {
		t.Fatalf("nil checker reported %v", d)
	}
}

func TestCheck_CreateCollides(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "Install Chrome"}}, nil)
	d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"install chrome "}, nil, ""))
	if !d.HasError() || len(d) != 1 {
		t.Fatalf("want one error, got %v", d)
	}
	detail := d[0].Detail()
	for _, want := range []string{`"install chrome "`, "(ID 7)", `id = "7"`, "to = <this resource's address>", "moved block"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail missing %q:\n%s", want, detail)
		}
	}
}

func TestCheck_WarnModeDowngrades(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "a"}}, nil)
	d := newChecker(ModeWarn, false).Check(context.Background(), request(ns, []string{"a"}, nil, ""))
	if d.HasError() || d.WarningsCount() != 1 {
		t.Fatalf("want one warning, got %v", d)
	}
}

func TestCheck_NoCollision(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "a"}}, nil)
	if d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"b"}, nil, "")); len(d) != 0 {
		t.Fatalf("unexpected diagnostics %v", d)
	}
}

func TestCheck_SelfExcluded(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "a"}}, nil)
	d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"A"}, []string{"b"}, "7"))
	if len(d) != 0 {
		t.Fatalf("an object must not collide with itself, got %v", d)
	}
}

func TestCheck_SelfIDOfAnotherTypeIsNotExcluded(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "7", Key: "a"}}, nil)
	ns.Sources[0].ResourceType = "jamfplatform_pro_other"
	d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"a"}, nil, "7"))
	if !d.HasError() {
		t.Fatal("an entry of a different resource type sharing the ID must still collide")
	}
	if !strings.Contains(d[0].Detail(), "jamfplatform_pro_other") {
		t.Errorf("detail should name the other resource type:\n%s", d[0].Detail())
	}
	if strings.Contains(d[0].Detail(), "import {") {
		t.Errorf("an import snippet for the wrong resource type must not be offered:\n%s", d[0].Detail())
	}
}

func TestCheck_UnchangedKeySkipped(t *testing.T) {
	var calls atomic.Int32
	ns := widgetNamespace([]Entry{{ID: "8", Key: "a"}}, &calls)
	d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"A "}, []string{"a"}, "7"))
	if len(d) != 0 || calls.Load() != 0 {
		t.Fatalf("a key unchanged under the collation must not list or report, got %v after %d calls", d, calls.Load())
	}
}

func TestCheck_RenameChecksNewKey(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "8", Key: "taken"}}, nil)
	d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"taken"}, []string{"old"}, "7"))
	if !d.HasError() {
		t.Fatal("a rename onto an existing key must collide")
	}
}

func TestCheck_EmptyKeySkipped(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "8", Key: ""}}, nil)
	if d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{""}, nil, "")); len(d) != 0 {
		t.Fatalf("an empty key is never a collision, got %v", d)
	}
}

func TestCheck_UnenforcedNeedsOptIn(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "8", Key: "a"}}, nil)
	ns.Enforced = false

	if d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"a"}, nil, "")); len(d) != 0 {
		t.Fatalf("an unenforced type must be skipped by default, got %v", d)
	}
	d := newChecker(ModeError, true).Check(context.Background(), request(ns, []string{"a"}, nil, ""))
	if !d.HasError() {
		t.Fatal("include_unenforced must check an unenforced type")
	}
	if !strings.Contains(d[0].Detail(), "accepts duplicates") {
		t.Errorf("detail should say Jamf accepts duplicates:\n%s", d[0].Detail())
	}
}

func TestCheck_SweepRunsOncePerNamespace(t *testing.T) {
	var calls atomic.Int32
	ns := widgetNamespace([]Entry{{ID: "8", Key: "a"}}, &calls)
	c := newChecker(ModeError, false)
	for _, key := range []string{"x", "y", "z"} {
		c.Check(context.Background(), request(ns, []string{key}, nil, ""))
	}
	if calls.Load() != 1 {
		t.Fatalf("want one listing, got %d", calls.Load())
	}
}

func TestCheck_ListFailureWarnsOnce(t *testing.T) {
	ns := widgetNamespace(nil, nil)
	ns.Sources[0].List = func(context.Context) ([]Entry, error) { return nil, errors.New("403 forbidden") }
	c := newChecker(ModeError, false)

	first := c.Check(context.Background(), request(ns, []string{"a"}, nil, ""))
	if first.HasError() || first.WarningsCount() != 1 {
		t.Fatalf("a failed listing must warn, not error: %v", first)
	}
	if second := c.Check(context.Background(), request(ns, []string{"b"}, nil, "")); len(second) != 0 {
		t.Fatalf("the notice must fire once per namespace, got %v", second)
	}
}

func TestCheck_SharedNamespaceSpansSources(t *testing.T) {
	ns := widgetNamespace(nil, nil)
	ns.Sources = append(ns.Sources, Source{
		Noun: "static widget",
		List: func(context.Context) ([]Entry, error) { return []Entry{{ID: "u-1", Key: "shared"}}, nil },
	})
	d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"shared"}, nil, ""))
	if !d.HasError() || !strings.Contains(d[0].Detail(), "static widget") {
		t.Fatalf("a collision in a second source must be reported, got %v", d)
	}
}

func TestCheck_EveryMatchReported(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "1", Key: "dup"}, {ID: "2", Key: "DUP"}}, nil)
	d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"dup"}, nil, ""))
	if len(d) != 2 {
		t.Fatalf("want a diagnostic per colliding object, got %d", len(d))
	}
}

func findNamespace(find func(context.Context, string) ([]Entry, error)) Namespace {
	return Namespace{
		ID:        "pro.finders",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: CollationAccentSensitive,
		Sources:   []Source{{Noun: "widget", ResourceType: testResourceType, Find: find}},
	}
}

func TestCheck_FindSourceQueriesPerKey(t *testing.T) {
	var calls atomic.Int32
	ns := findNamespace(func(_ context.Context, key string) ([]Entry, error) {
		calls.Add(1)
		if key == "taken" {
			return []Entry{{ID: "9", Key: "Taken"}}, nil
		}
		return nil, nil
	})
	c := newChecker(ModeError, false)

	if d := c.Check(context.Background(), request(ns, []string{"free"}, nil, "")); len(d) != 0 {
		t.Fatalf("a key with no candidates must not collide, got %v", d)
	}
	d := c.Check(context.Background(), request(ns, []string{"taken"}, nil, ""))
	if !d.HasError() || !strings.Contains(d[0].Detail(), "(ID 9)") {
		t.Fatalf("want a collision with ID 9, got %v", d)
	}
	if calls.Load() != 2 {
		t.Fatalf("want one lookup per key, got %d", calls.Load())
	}
}

func TestCheck_FindCandidatesAreComparedUnderTheCollation(t *testing.T) {
	ns := findNamespace(func(context.Context, string) ([]Entry, error) {
		return []Entry{{ID: "9", Key: "Café"}}, nil
	})
	if d := newChecker(ModeError, false).Check(context.Background(), request(ns, []string{"Cafe"}, nil, "")); len(d) != 0 {
		t.Fatalf("an accent-sensitive namespace must not report Café for Cafe, got %v", d)
	}
}

func TestCheck_FindResultIsMemoisedPerKey(t *testing.T) {
	var calls atomic.Int32
	ns := findNamespace(func(context.Context, string) ([]Entry, error) {
		calls.Add(1)
		return nil, nil
	})
	c := newChecker(ModeError, false)
	c.Check(context.Background(), request(ns, []string{"a"}, nil, ""))
	c.Check(context.Background(), request(ns, []string{"A "}, nil, ""))
	if calls.Load() != 1 {
		t.Fatalf("keys equal under the collation must share one lookup, got %d", calls.Load())
	}
}

func TestCheck_FindFailureWarnsOnce(t *testing.T) {
	ns := findNamespace(func(context.Context, string) ([]Entry, error) { return nil, errors.New("boom") })
	c := newChecker(ModeError, false)
	first := c.Check(context.Background(), request(ns, []string{"a"}, nil, ""))
	if first.HasError() || first.WarningsCount() != 1 {
		t.Fatalf("a failed lookup must warn, not error: %v", first)
	}
	if second := c.Check(context.Background(), request(ns, []string{"b"}, nil, "")); len(second) != 0 {
		t.Fatalf("the notice must fire once per namespace, got %v", second)
	}
}

func TestCheck_ListAndFindSourcesMix(t *testing.T) {
	ns := widgetNamespace([]Entry{{ID: "1", Key: "listed"}}, nil)
	ns.Sources = append(ns.Sources, Source{
		Noun: "found widget",
		Find: func(context.Context, string) ([]Entry, error) { return []Entry{{ID: "2", Key: "found"}}, nil },
	})
	c := newChecker(ModeError, false)
	if d := c.Check(context.Background(), request(ns, []string{"listed"}, nil, "")); !d.HasError() {
		t.Fatal("the listed source must still be swept")
	}
	if d := c.Check(context.Background(), request(ns, []string{"found"}, nil, "")); !d.HasError() {
		t.Fatal("the find source must also be consulted")
	}
}
