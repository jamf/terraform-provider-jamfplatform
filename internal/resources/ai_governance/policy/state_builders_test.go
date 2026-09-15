// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"
)

// detailFixture is the policy the EU sandbox returned for a published Claude Code policy on
// 2026-08-30, with the actor identifiers dropped.
func detailFixture() *aigovernance.PolicyDetail {
	description := "wire probe"
	version := 1
	return &aigovernance.PolicyDetail{
		ID:                   "86402bb9-bc6f-405c-8f6f-84cf09380692",
		Name:                 "tf-probe-lifecycle",
		Description:          &description,
		ToolID:               "com.anthropic.claudecode",
		SchemaVersion:        "2026-08-14",
		SchemaDrift:          false,
		Settings:             json.RawMessage(`{"model":"sonnet","verbose":true}`),
		CurrentVersionNumber: &version,
		HasDraft:             false,
		Status:               aigovernance.PolicyDetailStatusActive,
		CreatedAt:            time.Date(2026, 8, 30, 9, 44, 7, 0, time.UTC),
		UpdatedAt:            time.Date(2026, 8, 30, 9, 44, 41, 0, time.UTC),
	}
}

func TestApplyPolicyToState(t *testing.T) {
	var model policyModel
	if err := applyPolicyToState(&model, detailFixture()); err != nil {
		t.Fatalf("applyPolicyToState: %v", err)
	}

	if model.ID.ValueString() != "86402bb9-bc6f-405c-8f6f-84cf09380692" {
		t.Errorf("id = %q", model.ID.ValueString())
	}
	if model.Name.ValueString() != "tf-probe-lifecycle" {
		t.Errorf("name = %q", model.Name.ValueString())
	}
	if model.Description.ValueString() != "wire probe" {
		t.Errorf("description = %q", model.Description.ValueString())
	}
	if model.ToolID.ValueString() != "com.anthropic.claudecode" {
		t.Errorf("tool_id = %q", model.ToolID.ValueString())
	}
	if model.SettingsJSON.ValueString() != `{"model":"sonnet","verbose":true}` {
		t.Errorf("settings kept verbatim? got %q", model.SettingsJSON.ValueString())
	}
	if model.PublishedVersion.ValueInt64() != 1 {
		t.Errorf("published_version = %d", model.PublishedVersion.ValueInt64())
	}
	if model.HasDraft.ValueBool() || model.SchemaDrift.ValueBool() {
		t.Error("has_draft and schema_drift should both be false for this fixture")
	}
	if model.CreatedAt.ValueString() != "2026-08-30T09:44:07Z" {
		t.Errorf("created_at = %q", model.CreatedAt.ValueString())
	}
	if model.UpdatedAt.ValueString() != "2026-08-30T09:44:41Z" {
		t.Errorf("updated_at = %q", model.UpdatedAt.ValueString())
	}
}

// TestApplyPolicyToStateNullables pins the two fields the platform returns as null: description when
// unset, and currentVersionNumber until the policy is first published.
func TestApplyPolicyToStateNullables(t *testing.T) {
	detail := detailFixture()
	detail.Description = nil
	detail.CurrentVersionNumber = nil
	detail.HasDraft = true

	var model policyModel
	if err := applyPolicyToState(&model, detail); err != nil {
		t.Fatalf("applyPolicyToState: %v", err)
	}
	if !model.Description.IsNull() {
		t.Errorf("description should be null, got %q", model.Description.ValueString())
	}
	if !model.PublishedVersion.IsNull() {
		t.Errorf("published_version should be null before the first publish, got %d", model.PublishedVersion.ValueInt64())
	}
	if !model.HasDraft.ValueBool() {
		t.Error("has_draft should be true for an unpublished policy")
	}
}

// TestApplyPolicyToStateBlankDescriptionIsNull pins that a description the platform stores as the
// empty string — the form an update sends to clear one, and the only form it clears on — reads back
// as null. Copying the blank through would put "" into state where the configuration says nothing
// and fail the apply that removed the attribute.
func TestApplyPolicyToStateBlankDescriptionIsNull(t *testing.T) {
	blank := ""
	detail := detailFixture()
	detail.Description = &blank

	var model policyModel
	if err := applyPolicyToState(&model, detail); err != nil {
		t.Fatalf("applyPolicyToState: %v", err)
	}
	if !model.Description.IsNull() {
		t.Errorf("description should be null, got %q", model.Description.ValueString())
	}
}

// TestApplyPolicyToStateKeepsAnExplicitBlankDescription pins the other side of that mapping: an
// operator who wrote description = "" keeps it, rather than having it collapsed to null and tripping
// the same inconsistency from the opposite direction.
func TestApplyPolicyToStateKeepsAnExplicitBlankDescription(t *testing.T) {
	blank := ""
	detail := detailFixture()
	detail.Description = &blank

	model := policyModel{Description: types.StringValue("")}
	if err := applyPolicyToState(&model, detail); err != nil {
		t.Fatalf("applyPolicyToState: %v", err)
	}
	if model.Description.IsNull() || model.Description.ValueString() != "" {
		t.Errorf("an explicitly blank description should be preserved, got %v", model.Description)
	}
}

func TestRenderSettings(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"object", `{"a":1}`, `{"a":1}`, false},
		{"empty body becomes an empty object", ``, `{}`, false},
		{"empty object", `{}`, `{}`, false},
		{"array is refused", `[]`, "", true},
		{"string is refused", `"x"`, "", true},
		{"unparseable is refused", `{oops`, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := renderSettings(json.RawMessage(c.raw))
			if c.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("renderSettings: %v", err)
			}
			if got.ValueString() != c.want {
				t.Errorf("got %q, want %q", got.ValueString(), c.want)
			}
		})
	}
}

// TestResolvePublish pins that the publish flag survives a refresh and lands on the schema default
// after an import, where there is no prior value at all. Getting this wrong makes the first plan
// after an import propose a change to an attribute the operator never wrote.
func TestResolvePublish(t *testing.T) {
	if got := resolvePublish(types.BoolNull()); !got.ValueBool() {
		t.Error("a null prior value must land on the schema default of true")
	}
	if got := resolvePublish(types.BoolUnknown()); !got.ValueBool() {
		t.Error("an unknown prior value must land on the schema default of true")
	}
	if got := resolvePublish(types.BoolValue(false)); got.ValueBool() {
		t.Error("an explicit false must be preserved")
	}
	if got := resolvePublish(types.BoolValue(true)); !got.ValueBool() {
		t.Error("an explicit true must be preserved")
	}
}
