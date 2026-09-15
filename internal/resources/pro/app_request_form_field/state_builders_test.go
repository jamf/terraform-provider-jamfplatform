// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_form_field

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignAppRequestFormFieldResourceModel(t *testing.T) {
	t.Run("full echo", func(t *testing.T) {
		var state AppRequestFormFieldResourceModel
		assignAppRequestFormFieldResourceModel(&state, &pro.AppRequestFormInputField{
			ID:          new(17),
			Title:       "Reason",
			Description: new("desc"),
			Priority:    3,
		})
		if state.ID.ValueString() != "17" {
			t.Errorf("id = %q", state.ID.ValueString())
		}
		if state.Title.ValueString() != "Reason" {
			t.Errorf("title = %q", state.Title.ValueString())
		}
		if state.Description.ValueString() != "desc" {
			t.Errorf("description = %q", state.Description.ValueString())
		}
		if state.Priority.ValueInt64() != 3 {
			t.Errorf("priority = %d", state.Priority.ValueInt64())
		}
	})

	t.Run("explicit empty description preserved (not collapsed to null)", func(t *testing.T) {
		var state AppRequestFormFieldResourceModel
		assignAppRequestFormFieldResourceModel(&state, &pro.AppRequestFormInputField{ID: new(1), Title: "T", Description: new(""), Priority: 0})
		if state.Description.IsNull() {
			t.Errorf("explicit empty description must round-trip as \"\", got null")
		}
		if state.Description.ValueString() != "" {
			t.Errorf("description = %q, want \"\"", state.Description.ValueString())
		}
	})

	t.Run("nil description -> null", func(t *testing.T) {
		var state AppRequestFormFieldResourceModel
		assignAppRequestFormFieldResourceModel(&state, &pro.AppRequestFormInputField{ID: new(1), Title: "T", Priority: 0})
		if !state.Description.IsNull() {
			t.Errorf("nil description must map to null, got %q", state.Description.ValueString())
		}
	})

	t.Run("nil ID does not clobber existing", func(t *testing.T) {
		state := AppRequestFormFieldResourceModel{ID: types.StringValue("99")}
		assignAppRequestFormFieldResourceModel(&state, &pro.AppRequestFormInputField{Title: "T", Priority: 1})
		if state.ID.ValueString() != "99" {
			t.Errorf("nil API ID must preserve existing id, got %q", state.ID.ValueString())
		}
	})
}
