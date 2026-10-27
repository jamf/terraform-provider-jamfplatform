// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package collision

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

func plannedChecker(ns Namespace) *Checker {
	c := newChecker(ModeError, false)
	c.noteRules(testResourceType, ns)
	return c
}

func TestCreateConflictHint(t *testing.T) {
	ns := widgetNamespace(nil, nil)
	conflict := &jamfplatform.APIResponseError{StatusCode: 409}

	hint := plannedChecker(ns).CreateConflictHint(conflict, testResourceType)
	if !strings.HasPrefix(hint, "\n\n") {
		t.Errorf("hint must start with a blank line so callers can append it: %q", hint)
	}
	for _, want := range []string{"Jamf Pro does not allow two objects with the same name", "Another resource in this configuration", "distinct value"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q:\n%s", want, hint)
		}
	}

	ns.KeyLabel = "display name"
	if got := plannedChecker(ns).CreateConflictHint(conflict, testResourceType); !strings.Contains(got, "display name") {
		t.Errorf("hint must name the namespace's key label:\n%s", got)
	}
}

func TestCreateConflictHint_NamesEveryEnforcedRule(t *testing.T) {
	a := widgetNamespace(nil, nil)
	b := widgetNamespace(nil, nil)
	b.ID, b.KeyLabel = "pro.widget-files", "file name"
	c := newChecker(ModeError, false)
	c.noteRules(testResourceType, a, b, a)
	got := c.CreateConflictHint(&jamfplatform.APIResponseError{StatusCode: 409}, testResourceType)
	if !strings.Contains(got, "name or file name") {
		t.Errorf("hint must name both attributes once each:\n%s", got)
	}
}

func TestCreateConflictHint_DuplicateWithoutA409(t *testing.T) {
	c := plannedChecker(widgetNamespace(nil, nil))
	dup := &jamfplatform.APIResponseError{StatusCode: 400, Errors: []jamfplatform.ErrorDetail{{Code: "DUPLICATE_FIELD", Field: "name", Description: "duplicate name"}}}
	if got := c.CreateConflictHint(dup, testResourceType); got == "" {
		t.Error("a 400 duplicate refusal must be explained")
	}
	other := &jamfplatform.APIResponseError{StatusCode: 400, Errors: []jamfplatform.ErrorDetail{{Code: "REQUIRED_FIELD", Field: "name", Description: "name is required"}}}
	if got := c.CreateConflictHint(other, testResourceType); got != "" {
		t.Errorf("an unrelated 400 must not be explained: %q", got)
	}
}

func TestCreateConflictHint_Silent(t *testing.T) {
	ns := widgetNamespace(nil, nil)
	c := plannedChecker(ns)
	conflict := &jamfplatform.APIResponseError{StatusCode: 409}
	if got := c.CreateConflictHint(nil, testResourceType); got != "" {
		t.Errorf("nil error: %q", got)
	}
	if got := c.CreateConflictHint(errors.New("boom"), testResourceType); got != "" {
		t.Errorf("non-API error: %q", got)
	}
	if got := c.CreateConflictHint(&jamfplatform.APIResponseError{StatusCode: 500}, testResourceType); got != "" {
		t.Errorf("non-409: %q", got)
	}
	if got := c.CreateConflictHint(conflict, "jamfplatform_pro_other"); got != "" {
		t.Errorf("a type never planned has no rule to name: %q", got)
	}
	ns.Enforced = false
	if got := plannedChecker(ns).CreateConflictHint(conflict, testResourceType); got != "" {
		t.Errorf("an unenforced namespace has no 409 to explain: %q", got)
	}
	var nilChecker *Checker
	if got := nilChecker.CreateConflictHint(conflict, testResourceType); got != "" {
		t.Errorf("nil checker: %q", got)
	}
}

func TestNoteManagedState(t *testing.T) {
	sch := schema.Schema{Attributes: map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true}}}
	typ := sch.Type().TerraformType(context.Background())
	state := func(id *string) *tfsdk.State {
		var v tftypes.Value
		if id == nil {
			v = tftypes.NewValue(typ, nil)
		} else {
			v = tftypes.NewValue(typ, map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, *id)})
		}
		return &tfsdk.State{Schema: sch, Raw: v}
	}
	id := "7"

	c := newChecker(ModeError, false)
	c.NoteManagedState(context.Background(), testResourceType, state(nil))
	if c.isManaged(Match{ID: "7", ResourceType: testResourceType}) {
		t.Fatal("a removed resource must not be recorded")
	}
	c.NoteManagedState(context.Background(), testResourceType, state(&id))
	if !c.isManaged(Match{ID: "7", ResourceType: testResourceType}) {
		t.Fatal("the state's id must be recorded")
	}

	var nilChecker *Checker
	nilChecker.NoteManagedState(context.Background(), testResourceType, state(&id))
	c.NoteManagedState(context.Background(), testResourceType, nil)
}
