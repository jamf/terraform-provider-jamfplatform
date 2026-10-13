// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var diskEncryptionObjType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"action":                           tftypes.String,
	"disk_encryption_configuration_id": tftypes.Number,
	"auth_restart":                     tftypes.Bool,
	"remediate_key_type":               tftypes.String,
	"remediate_disk_encryption_configuration_id": tftypes.Number,
}}

func diskEncryptionSchema() schema.Schema {
	return schema.Schema{Attributes: map[string]schema.Attribute{
		"disk_encryption": schema.SingleNestedAttribute{
			Optional: true,
			Attributes: map[string]schema.Attribute{
				"action":                           schema.StringAttribute{Optional: true},
				"disk_encryption_configuration_id": schema.Int64Attribute{Optional: true},
				"auth_restart":                     schema.BoolAttribute{Optional: true},
				"remediate_key_type":               schema.StringAttribute{Optional: true},
				"remediate_disk_encryption_configuration_id": schema.Int64Attribute{Optional: true},
			},
		},
	}}
}

type diskEncryptionConfig struct {
	action, keyType       any
	configID, remediateID any
	authRestart           any
}

func (c diskEncryptionConfig) config(ctx context.Context, t *testing.T) tfsdk.Config {
	t.Helper()
	s := diskEncryptionSchema()
	block := tftypes.NewValue(diskEncryptionObjType, map[string]tftypes.Value{
		"action":                           tftypes.NewValue(tftypes.String, c.action),
		"disk_encryption_configuration_id": tftypes.NewValue(tftypes.Number, c.configID),
		"auth_restart":                     tftypes.NewValue(tftypes.Bool, c.authRestart),
		"remediate_key_type":               tftypes.NewValue(tftypes.String, c.keyType),
		"remediate_disk_encryption_configuration_id": tftypes.NewValue(tftypes.Number, c.remediateID),
	})
	raw := tftypes.NewValue(s.Type().TerraformType(ctx).(tftypes.Object), map[string]tftypes.Value{"disk_encryption": block})
	return tfsdk.Config{Schema: s, Raw: raw}
}

func TestDiskEncryptionCompanions(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		cfg     diskEncryptionConfig
		wantErr string
	}{
		{"apply without an id is refused", diskEncryptionConfig{action: "apply"}, "disk_encryption_configuration_id"},
		{"apply with id 0 is refused", diskEncryptionConfig{action: "apply", configID: 0}, "disk_encryption_configuration_id"},
		{"apply with an id is accepted", diskEncryptionConfig{action: "apply", configID: 5, authRestart: true}, ""},
		{"apply with a remediate key type is dropped by Jamf Pro", diskEncryptionConfig{action: "apply", configID: 5, keyType: "Institutional"}, "remediate_key_type"},
		{"remediate individual needs nothing", diskEncryptionConfig{action: "remediate"}, ""},
		{"remediate individual with an id: id is dropped", diskEncryptionConfig{action: "remediate", keyType: "Individual", remediateID: 7}, "remediate_disk_encryption_configuration_id"},
		{"remediate institutional without an id is refused", diskEncryptionConfig{action: "remediate", keyType: "Institutional"}, "remediate_disk_encryption_configuration_id"},
		{"remediate both without an id is refused", diskEncryptionConfig{action: "remediate", keyType: "Individual And Institutional"}, "remediate_disk_encryption_configuration_id"},
		{"remediate institutional with an id is accepted", diskEncryptionConfig{action: "remediate", keyType: "Institutional", remediateID: 7}, ""},
		{"remediate with the apply id is dropped", diskEncryptionConfig{action: "remediate", configID: 5}, "disk_encryption_configuration_id"},
		{"remediate with auth_restart true is dropped", diskEncryptionConfig{action: "remediate", authRestart: true}, "auth_restart"},
		{"remediate with auth_restart false is harmless", diskEncryptionConfig{action: "remediate", authRestart: false}, ""},
		{"none with an apply id is dropped", diskEncryptionConfig{action: "none", configID: 5}, "disk_encryption_configuration_id"},
		{"none with nothing is accepted", diskEncryptionConfig{action: "none"}, ""},
		{"no action configured is left alone", diskEncryptionConfig{configID: 5, authRestart: true}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := resource.ValidateConfigResponse{}
			diskEncryptionCompanionsValidator{}.ValidateResource(ctx, resource.ValidateConfigRequest{Config: tc.cfg.config(ctx, t)}, &resp)
			if tc.wantErr == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected error: %v", resp.Diagnostics)
				}
				return
			}
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error naming %q, got none", tc.wantErr)
			}
			want := path.Root("disk_encryption").AtName(tc.wantErr)
			for _, d := range resp.Diagnostics.Errors() {
				if dp, ok := d.(diag.DiagnosticWithPath); ok && dp.Path().Equal(want) {
					return
				}
			}
			t.Errorf("no error on %s: %v", want, resp.Diagnostics)
		})
	}
}

func efiPasswordPlanRequest(ctx context.Context, t *testing.T, mode, password any, create bool) resource.ModifyPlanRequest {
	t.Helper()
	s := schema.Schema{Attributes: map[string]schema.Attribute{
		"efi_password": schema.SingleNestedAttribute{
			Optional: true,
			Attributes: map[string]schema.Attribute{
				"of_mode":     schema.StringAttribute{Optional: true},
				"of_password": schema.StringAttribute{Optional: true, WriteOnly: true},
			},
		},
	}}
	obj := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"of_mode": tftypes.String, "of_password": tftypes.String}}
	root := s.Type().TerraformType(ctx).(tftypes.Object)
	raw := tftypes.NewValue(root, map[string]tftypes.Value{"efi_password": tftypes.NewValue(obj, map[string]tftypes.Value{
		"of_mode":     tftypes.NewValue(tftypes.String, mode),
		"of_password": tftypes.NewValue(tftypes.String, password),
	})})
	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: s, Raw: raw},
		Plan:   tfsdk.Plan{Schema: s, Raw: raw},
		State:  tfsdk.State{Schema: s, Raw: raw},
	}
	if create {
		req.State.Raw = tftypes.NewValue(root, nil)
	}
	return req
}

func TestValidateEFIPasswordOnCreate(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		mode     any
		password any
		create   bool
		wantErr  bool
	}{
		{"command without a password on create is refused", "command", nil, true, true},
		{"command with an empty password on create is refused", "command", "", true, true},
		{"command with a password on create is accepted", "command", "Abcdef12", true, false},
		{"command without a password on update retains the stored one", "command", nil, false, false},
		{"mode none needs no password", "none", nil, true, false},
		{"no mode configured is left alone", nil, nil, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := resource.ModifyPlanResponse{}
			validateEFIPasswordOnCreate(ctx, efiPasswordPlanRequest(ctx, t, tc.mode, tc.password, tc.create), &resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v (%v)", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}
