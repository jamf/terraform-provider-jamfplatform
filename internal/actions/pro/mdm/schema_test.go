// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func assertAttrsPresent(t *testing.T, schema func(context.Context, action.SchemaRequest, *action.SchemaResponse), want []string) {
	t.Helper()
	var resp action.SchemaResponse
	schema(context.Background(), action.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range want {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

func assertRequired(t *testing.T, schema func(context.Context, action.SchemaRequest, *action.SchemaResponse), required []string) {
	t.Helper()
	var resp action.SchemaResponse
	schema(context.Background(), action.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range required {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("missing attribute %q", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}
}

func assertTypeName(t *testing.T, meta func(context.Context, action.MetadataRequest, *action.MetadataResponse), want string) {
	t.Helper()
	req := action.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp action.MetadataResponse
	meta(context.Background(), req, &resp)
	if resp.TypeName != want {
		t.Errorf("expected type name %q, got %q", want, resp.TypeName)
	}
}

// --- send_blank_push ---

func TestSendBlankPushAction_Metadata(t *testing.T) {
	assertTypeName(t, NewSendBlankPushAction().(*SendBlankPushAction).Metadata, "jamfplatform_pro_send_blank_push")
}

func TestSendBlankPushAction_Schema(t *testing.T) {
	assertAttrsPresent(t, NewSendBlankPushAction().(*SendBlankPushAction).Schema,
		[]string{"management_ids", "serial_numbers"})
}

// --- renew_mdm_profile ---

func TestRenewMdmProfileAction_Metadata(t *testing.T) {
	assertTypeName(t, NewRenewMdmProfileAction().(*RenewMdmProfileAction).Metadata, "jamfplatform_pro_renew_mdm_profile")
}

func TestRenewMdmProfileAction_Schema(t *testing.T) {
	schema := NewRenewMdmProfileAction().(*RenewMdmProfileAction).Schema
	assertAttrsPresent(t, schema, []string{"udids"})
	assertRequired(t, schema, []string{"udids"})
}

// --- flush_mdm_commands ---

func TestFlushMdmCommandsAction_Metadata(t *testing.T) {
	assertTypeName(t, NewFlushMdmCommandsAction().(*FlushMdmCommandsAction).Metadata, "jamfplatform_pro_flush_mdm_commands")
}

func TestFlushMdmCommandsAction_Schema(t *testing.T) {
	schema := NewFlushMdmCommandsAction().(*FlushMdmCommandsAction).Schema
	assertAttrsPresent(t, schema, []string{"id_type", "id", "status"})
	assertRequired(t, schema, []string{"id_type", "id", "status"})
}

// --- plan-time device-target validation ---

// TestSendBlankPushValidatorsWired covers the at-least-one-of rule: the action
// accepts management_ids and/or serial_numbers, and previously only rejected
// "neither" once the apply was already running.
//
// It asserts which selectors the validator names, not merely that some validator
// is present. The behavioural tests for this rule are tenant-gated acceptance
// tests, so a non-empty check is the whole guard in CI — and it would pass just
// as happily if the re-pointing onto send_blank_push had picked up an unrelated
// validator when the twelve batch actions it used to ride were removed.
func TestSendBlankPushValidatorsWired(t *testing.T) {
	a, ok := NewSendBlankPushAction().(action.ActionWithConfigValidators)
	if !ok {
		t.Fatal("send_blank_push declares no ConfigValidators")
	}
	validators := a.ConfigValidators(context.Background())
	if len(validators) == 0 {
		t.Fatal("send_blank_push declares an empty ConfigValidators slice")
	}
	var described strings.Builder
	for _, v := range validators {
		described.WriteString(v.Description(context.Background()))
		described.WriteString("\n")
	}
	for _, selector := range []string{"management_ids", "serial_numbers"} {
		if !strings.Contains(described.String(), selector) {
			t.Errorf("no ConfigValidator names %q; the at-least-one-of rule is not wired to both selectors:\n%s", selector, described.String())
		}
	}
}

// --- lost mode ---

func TestLostModeActions_Metadata(t *testing.T) {
	assertTypeName(t, NewEnableLostModeAction().(*EnableLostModeAction).Metadata, "jamfplatform_pro_enable_lost_mode")
	assertTypeName(t, NewDisableLostModeAction().(*DisableLostModeAction).Metadata, "jamfplatform_pro_disable_lost_mode")
	assertTypeName(t, NewPlayLostModeSoundAction().(*PlayLostModeSoundAction).Metadata, "jamfplatform_pro_play_lost_mode_sound")
}

func TestEnableLostModeAction_Schema(t *testing.T) {
	schema := NewEnableLostModeAction().(*EnableLostModeAction).Schema
	assertAttrsPresent(t, schema, []string{"management_ids", "serial_numbers", "message", "phone_number", "footnote", "timeouts"})

	var resp action.SchemaResponse
	schema(context.Background(), action.SchemaRequest{}, &resp)
	for _, name := range []string{"message", "phone_number", "footnote"} {
		if !resp.Schema.Attributes[name].IsOptional() {
			t.Errorf("%s must be optional", name)
		}
	}
}

func TestEnableLostModeAction_AtLeastOneOfMessageOrPhone(t *testing.T) {
	var described strings.Builder
	for _, v := range NewEnableLostModeAction().(action.ActionWithConfigValidators).ConfigValidators(context.Background()) {
		described.WriteString(v.Description(context.Background()) + "\n")
	}
	// A footnote alone is refused by Jamf Pro, so the rule must name exactly the
	// two attributes that satisfy it and not the footnote.
	for _, want := range []string{"message", "phone_number"} {
		if !strings.Contains(described.String(), want) {
			t.Errorf("no ConfigValidator names %q:\n%s", want, described.String())
		}
	}
}

func TestEnableLostModeAction_DocumentsTheDuplicateCommandTrap(t *testing.T) {
	desc := schemaOf(t, NewEnableLostModeAction()).MarkdownDescription
	if !strings.Contains(desc, "already has this command waiting") {
		t.Errorf("the pending-command behaviour is not documented:\n%s", desc)
	}
}

func TestHasVisibleText_RejectsBlank(t *testing.T) {
	for value, wantErr := range map[string]bool{"Call me": false, " x ": false, "   ": true, "\t\n": true} {
		var resp validatorResponse
		validateString(hasVisibleText, value, &resp)
		if resp.hasError != wantErr {
			t.Errorf("hasVisibleText(%q) error = %v, want %v", value, resp.hasError, wantErr)
		}
	}
}

// TestLostModeCommands_MarshalTheWireBody pins the request body the SDK's union
// marshaller produces against what was accepted on the wire (2026-10-22).
func TestLostModeCommands_MarshalTheWireBody(t *testing.T) {
	message, phone := "Call me", "+15555550100"
	cases := map[string]struct {
		got  *pro.MDMCommandRequestCommandData
		want string
	}{
		"enable, message and phone": {
			enableLostModeCommand(EnableLostModeActionModel{Message: types.StringValue(message), PhoneNumber: types.StringValue(phone)}),
			`{"commandType":"ENABLE_LOST_MODE","lostModeMessage":"Call me","lostModePhone":"+15555550100"}`,
		},
		"enable, unset fields are omitted": {
			enableLostModeCommand(EnableLostModeActionModel{Message: types.StringValue(message)}),
			`{"commandType":"ENABLE_LOST_MODE","lostModeMessage":"Call me"}`,
		},
		"disable": {disableLostModeCommand(), `{"commandType":"DISABLE_LOST_MODE"}`},
		"play":    {playLostModeSoundCommand(), `{"commandType":"PLAY_LOST_MODE_SOUND"}`},
	}
	for name, c := range cases {
		out, err := json.Marshal(c.got)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if string(out) != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, out, c.want)
		}
	}
}

type validatorResponse struct{ hasError bool }

func validateString(v validator.String, value string, out *validatorResponse) {
	var resp validator.StringResponse
	v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue(value)}, &resp)
	out.hasError = resp.Diagnostics.HasError()
}
