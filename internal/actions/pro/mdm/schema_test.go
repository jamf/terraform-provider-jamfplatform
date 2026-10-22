// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
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

// --- set_wallpaper ---

func TestSetWallpaperAction_Metadata(t *testing.T) {
	assertTypeName(t, NewSetWallpaperAction().(*SetWallpaperAction).Metadata, "jamfplatform_pro_set_wallpaper")
}

func TestSetWallpaperAction_Schema(t *testing.T) {
	schema := NewSetWallpaperAction().(*SetWallpaperAction).Schema
	assertAttrsPresent(t, schema, []string{"management_ids", "serial_numbers", "location", "image", "icon_id", "timeouts"})
	assertRequired(t, schema, []string{"location"})
}

// TestSetWallpaperAction_SelectorsDescribeTheirOwnLookup guards the one place the
// shared selector text had to change: this command takes serial numbers as they
// are and resolves Management IDs, the reverse of every other command.
func TestSetWallpaperAction_SelectorsDescribeTheirOwnLookup(t *testing.T) {
	schema := schemaOf(t, NewSetWallpaperAction())
	if got := schema.Attributes["serial_numbers"].GetMarkdownDescription(); strings.Contains(got, "looked up") {
		t.Errorf("serial_numbers is sent as written, but its description says it is looked up:\n%s", got)
	}
	if got := schema.Attributes["management_ids"].GetMarkdownDescription(); !strings.Contains(got, "serial number before the command") {
		t.Errorf("management_ids must say it is resolved to a serial number:\n%s", got)
	}
}

func TestSetWallpaperAction_ExactlyOneImageSource(t *testing.T) {
	var described strings.Builder
	for _, v := range NewSetWallpaperAction().(action.ActionWithConfigValidators).ConfigValidators(context.Background()) {
		described.WriteString(v.Description(context.Background()) + "\n")
	}
	for _, want := range []string{"image", "icon_id", "management_ids", "serial_numbers"} {
		if !strings.Contains(described.String(), want) {
			t.Errorf("no ConfigValidator names %q:\n%s", want, described.String())
		}
	}
}

func TestWallpaperLocations_AreTheSDKValues(t *testing.T) {
	want := map[string]int{"lock_screen": 1, "home_screen": 2, "lock_and_home_screens": 3}
	if len(wallpaperLocations) != len(want) {
		t.Fatalf("wallpaperLocations has %d entries, want %d", len(wallpaperLocations), len(want))
	}
	for name, v := range want {
		if got := wallpaperLocations[name]; got != v {
			t.Errorf("%s = %d, want %d", name, got, v)
		}
	}
	sdk := map[int]bool{}
	for _, v := range proclassic.MobileDeviceCommandPostGeneralWallpaperSettingValues() {
		sdk[v] = true
	}
	for name, v := range wallpaperLocations {
		if !sdk[v] {
			t.Errorf("%s maps to %d, which the SDK does not list", name, v)
		}
	}
}

var onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestWallpaperImage(t *testing.T) {
	jpeg := base64.StdEncoding.EncodeToString([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0})
	text := base64.StdEncoding.EncodeToString([]byte("not an image"))
	cases := map[string]bool{
		onePixelPNG:      false,
		jpeg:             false,
		text:             true,
		"%%% not base64": true,
		"":               true,
	}
	for value, wantErr := range cases {
		var resp validatorResponse
		validateString(wallpaperImage{}, value, &resp)
		if resp.hasError != wantErr {
			t.Errorf("wallpaperImage(%.20q) error = %v, want %v", value, resp.hasError, wantErr)
		}
	}
}

func TestWallpaperImage_UnknownIsSkipped(t *testing.T) {
	var resp validator.StringResponse
	wallpaperImage{}.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringUnknown()}, &resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("an unknown image must wait until it is known: %v", resp.Diagnostics)
	}
}

// TestSetWallpaperRequest_NamesDevicesBySerialInTheBody pins the wire shape
// probed on 2026-10-22: serial_number matches a device, management_id does not.
func TestSetWallpaperRequest_NamesDevicesBySerialInTheBody(t *testing.T) {
	data := SetWallpaperActionModel{
		Location: types.StringValue("lock_and_home_screens"),
		Image:    types.StringValue(onePixelPNG),
		IconID:   types.StringNull(),
	}
	req, err := setWallpaperRequest(data, []string{"AAA111", "BBB222"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := xml.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{
		"<command>Wallpaper</command>",
		"<wallpaper_setting>3</wallpaper_setting>",
		"<wallpaper_content>" + onePixelPNG + "</wallpaper_content>",
		"<serial_number>AAA111</serial_number>",
		"<serial_number>BBB222</serial_number>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("request body missing %s:\n%s", want, got)
		}
	}
	for _, banned := range []string{"management_id", "wallpaper_id"} {
		if strings.Contains(got, banned) {
			t.Errorf("request body must not carry %s:\n%s", banned, got)
		}
	}
}

func TestSetWallpaperRequest_IconID(t *testing.T) {
	data := SetWallpaperActionModel{Location: types.StringValue("home_screen"), Image: types.StringNull(), IconID: types.StringValue("42")}
	req, err := setWallpaperRequest(data, []string{"AAA111"})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := xml.Marshal(req)
	for _, want := range []string{"<wallpaper_id>42</wallpaper_id>", "<wallpaper_setting>2</wallpaper_setting>"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("request body missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(string(out), "wallpaper_content") {
		t.Errorf("an icon id must not send image content:\n%s", out)
	}
}

func TestUniqueStrings_KeepsFirstSeenOrder(t *testing.T) {
	got := uniqueStrings([]string{"b", "a", "b", "c", "a"})
	want := []string{"b", "a", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("uniqueStrings = %v, want %v", got, want)
	}
}

func TestPositiveInteger(t *testing.T) {
	for value, want := range map[string]bool{"1": true, "5227": true, "0": false, "-1": false, "01": false, "1.5": false, "": false, "abc": false} {
		if got := positiveInteger.MatchString(value); got != want {
			t.Errorf("positiveInteger(%q) = %v, want %v", value, got, want)
		}
	}
}

// --- tranche 3: single-device and secret-carrying commands ---

func TestTranche3Actions_Metadata(t *testing.T) {
	assertTypeName(t, NewClearPasscodeAction().(*ClearPasscodeAction).Metadata, "jamfplatform_pro_clear_passcode")
	assertTypeName(t, NewSetAutoAdminPasswordAction().(*SetAutoAdminPasswordAction).Metadata, "jamfplatform_pro_set_auto_admin_password")
	assertTypeName(t, NewSetRecoveryLockAction().(*SetRecoveryLockAction).Metadata, "jamfplatform_pro_set_recovery_lock")
	assertTypeName(t, NewClearRecoveryLockAction().(*ClearRecoveryLockAction).Metadata, "jamfplatform_pro_clear_recovery_lock")
}

// TestSingleActions_TakeOneDevice guards the asymmetry with the batch actions:
// a payload with a per-device value must not accept a list, or one device's
// value would be applied to all of them.
func TestSingleActions_TakeOneDevice(t *testing.T) {
	for name, a := range singleActions() {
		schema := schemaOf(t, a)
		for _, attr := range []string{"management_id", "serial_number"} {
			if _, ok := schema.Attributes[attr]; !ok {
				t.Errorf("%s: missing %q", name, attr)
			}
		}
		for _, gone := range []string{"management_ids", "serial_numbers"} {
			if _, ok := schema.Attributes[gone]; ok {
				t.Errorf("%s: carries a per-device value but accepts the list %q", name, gone)
			}
		}
		if !strings.Contains(schema.MarkdownDescription, "targets one device") {
			t.Errorf("%s: does not say it targets one device", name)
		}
		validators := a.(action.ActionWithConfigValidators).ConfigValidators(context.Background())
		var described strings.Builder
		for _, v := range validators {
			described.WriteString(v.Description(context.Background()))
		}
		for _, selector := range []string{"management_id", "serial_number"} {
			if !strings.Contains(described.String(), selector) {
				t.Errorf("%s: no ConfigValidator names %q", name, selector)
			}
		}
	}
}

func TestSecretAttributes_SayTheyAppearInPlanOutput(t *testing.T) {
	cases := map[string][]string{
		"set_recovery_lock":       {"password"},
		"device_lock":             {"pin"},
		"set_auto_admin_password": {"password"},
		"clear_passcode":          {"unlock_token"},
	}
	actions := map[string]action.Action{
		"set_recovery_lock":       NewSetRecoveryLockAction(),
		"device_lock":             NewDeviceLockAction(),
		"set_auto_admin_password": NewSetAutoAdminPasswordAction(),
		"clear_passcode":          NewClearPasscodeAction(),
	}
	for name, attrs := range cases {
		schema := schemaOf(t, actions[name])
		for _, attr := range attrs {
			if desc := schema.Attributes[attr].GetMarkdownDescription(); !strings.Contains(desc, "appears in Terraform plan output") {
				t.Errorf("%s.%s carries a secret without saying it is plan-visible:\n%s", name, attr, desc)
			}
		}
	}
}

func TestSetAutoAdminPasswordAction_RequiresBothFields(t *testing.T) {
	assertRequired(t, NewSetAutoAdminPasswordAction().(*SetAutoAdminPasswordAction).Schema, []string{"guid", "password"})
}

func TestSetRecoveryLockAction_RequiresAPassword(t *testing.T) {
	assertRequired(t, NewSetRecoveryLockAction().(*SetRecoveryLockAction).Schema, []string{"password"})
	assertAttrsPresent(t, NewClearRecoveryLockAction().(*ClearRecoveryLockAction).Schema, []string{"management_ids", "serial_numbers"})
	if _, ok := schemaOf(t, NewClearRecoveryLockAction()).Attributes["password"]; ok {
		t.Error("clear_recovery_lock must take no password")
	}
}

func TestTranche3Commands_MarshalTheWireBody(t *testing.T) {
	cases := map[string]struct {
		got  *pro.MDMCommandRequestCommandData
		want string
	}{
		"clear passcode":    {clearPasscodeCommand("dG9rZW4="), `{"commandType":"CLEAR_PASSCODE","unlockToken":"dG9rZW4="}`},
		"set recovery lock": {setRecoveryLockCommand("s3cret"), `{"commandType":"SET_RECOVERY_LOCK","newPassword":"s3cret"}`},
		// An empty password clears the lock, so it must be sent, not omitted.
		"clear recovery lock": {setRecoveryLockCommand(""), `{"commandType":"SET_RECOVERY_LOCK","newPassword":""}`},
		"set auto admin password": {
			setAutoAdminPasswordCommand(SetAutoAdminPasswordActionModel{Guid: types.StringValue("G-1"), Password: types.StringValue("pw")}),
			`{"commandType":"SET_AUTO_ADMIN_PASSWORD","guid":"G-1","password":"pw"}`,
		},
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

// --- tranche 2: destructive commands ---

func TestDestructiveActions_Metadata(t *testing.T) {
	assertTypeName(t, NewDeviceLockAction().(*DeviceLockAction).Metadata, "jamfplatform_pro_device_lock")
	assertTypeName(t, NewDeleteUserAction().(*DeleteUserAction).Metadata, "jamfplatform_pro_delete_user")
}

func TestDeviceLockAction_PinMustBeSixCharacters(t *testing.T) {
	pin := schemaOf(t, NewDeviceLockAction()).Attributes["pin"].(actionschema.StringAttribute)
	for value, wantErr := range map[string]bool{"123456": false, "abcdef": false, "12345": true, "1234567": true, "": true} {
		var resp validator.StringResponse
		for _, v := range pin.Validators {
			v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue(value)}, &resp)
		}
		if resp.Diagnostics.HasError() != wantErr {
			t.Errorf("pin %q error = %v, want %v", value, resp.Diagnostics.HasError(), wantErr)
		}
	}
}

func TestDeleteUserAction_ExactlyOneOfUserOrAll(t *testing.T) {
	var described strings.Builder
	for _, v := range NewDeleteUserAction().(action.ActionWithConfigValidators).ConfigValidators(context.Background()) {
		described.WriteString(v.Description(context.Background()) + "\n")
	}
	for _, want := range []string{"user_name", "delete_all_users", "management_ids", "serial_numbers"} {
		if !strings.Contains(described.String(), want) {
			t.Errorf("no ConfigValidator names %q:\n%s", want, described.String())
		}
	}
}

func TestDeleteUserAction_DeleteAllCanOnlyBeTrue(t *testing.T) {
	attr := schemaOf(t, NewDeleteUserAction()).Attributes["delete_all_users"].(actionschema.BoolAttribute)
	for value, wantErr := range map[bool]bool{true: false, false: true} {
		var resp validator.BoolResponse
		for _, v := range attr.Validators {
			v.ValidateBool(context.Background(), validator.BoolRequest{ConfigValue: types.BoolValue(value)}, &resp)
		}
		if resp.Diagnostics.HasError() != wantErr {
			t.Errorf("delete_all_users = %v error = %v, want %v", value, resp.Diagnostics.HasError(), wantErr)
		}
	}
}

func TestDeleteUserAction_WarnsThatARepeatCommandIsDropped(t *testing.T) {
	desc := schemaOf(t, NewDeleteUserAction()).MarkdownDescription
	if !strings.Contains(desc, "already has this command waiting") {
		t.Errorf("the pending-command behaviour is not documented:\n%s", desc)
	}
}

func TestDestructiveCommands_MarshalTheWireBody(t *testing.T) {
	cases := map[string]struct {
		got  *pro.MDMCommandRequestCommandData
		want string
	}{
		"lock, everything": {
			deviceLockCommand(DeviceLockActionModel{Message: types.StringValue("m"), PhoneNumber: types.StringValue("+1 555 0100"), Pin: types.StringValue("123456")}),
			`{"commandType":"DEVICE_LOCK","message":"m","phoneNumber":"+1 555 0100","pin":"123456"}`,
		},
		"lock, nothing": {deviceLockCommand(DeviceLockActionModel{}), `{"commandType":"DEVICE_LOCK"}`},
		"delete one user": {
			deleteUserCommand(DeleteUserActionModel{UserName: types.StringValue("alex"), ForceDeletion: types.BoolValue(true)}),
			`{"commandType":"DELETE_USER","forceDeletion":true,"userName":"alex"}`,
		},
		"delete all users": {
			deleteUserCommand(DeleteUserActionModel{DeleteAllUsers: types.BoolValue(true)}),
			`{"commandType":"DELETE_USER","deleteAllUsers":true}`,
		},
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
