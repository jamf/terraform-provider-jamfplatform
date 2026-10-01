// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCheckUsernameRequiresBasic(t *testing.T) {
	tests := []struct {
		name     string
		username types.String
		auth     types.String
		wantErr  bool
	}{
		{"username with basic ok", types.StringValue("bob"), types.StringValue(authTypeBasic), false},
		{"username with none rejected", types.StringValue("bob"), types.StringValue(authTypeNone), true},
		{"username with header rejected", types.StringValue("bob"), types.StringValue(authTypeHeader), true},
		{"no username ok", types.StringNull(), types.StringValue(authTypeNone), false},
		{"auth unknown skips", types.StringValue("bob"), types.StringUnknown(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rv := checkUsernameRequiresBasic(WebhookResourceModel{Username: tt.username, AuthenticationType: tt.auth})
			if (rv != nil) != tt.wantErr {
				t.Errorf("got violation=%v, want=%v", rv != nil, tt.wantErr)
			}
		})
	}
}

func TestCheckPasswordRequiresBasicOrHash(t *testing.T) {
	tests := []struct {
		name    string
		pw      types.String
		auth    types.String
		wantErr bool
	}{
		{"password with basic ok", types.StringValue("p"), types.StringValue(authTypeBasic), false},
		{"password with hash ok", types.StringValue("p"), types.StringValue(authTypeHashSignature), false},
		{"password with none rejected", types.StringValue("p"), types.StringValue(authTypeNone), true},
		{"password with header rejected", types.StringValue("p"), types.StringValue(authTypeHeader), true},
		{"no password ok", types.StringNull(), types.StringValue(authTypeHeader), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rv := checkPasswordRequiresBasicOrHash(WebhookResourceModel{Password: tt.pw, AuthenticationType: tt.auth})
			if (rv != nil) != tt.wantErr {
				t.Errorf("got violation=%v, want=%v", rv != nil, tt.wantErr)
			}
		})
	}
}

func TestCheckHeaderRequiresHeaderAuth(t *testing.T) {
	tests := []struct {
		name    string
		header  types.String
		auth    types.String
		wantErr bool
	}{
		{"header with header-auth ok", types.StringValue("{}"), types.StringValue(authTypeHeader), false},
		{"header with none rejected", types.StringValue("{}"), types.StringValue(authTypeNone), true},
		{"header with basic rejected", types.StringValue("{}"), types.StringValue(authTypeBasic), true},
		{"no header ok", types.StringNull(), types.StringValue(authTypeBasic), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rv := checkHeaderRequiresHeaderAuth(WebhookResourceModel{Header: tt.header, AuthenticationType: tt.auth})
			if (rv != nil) != tt.wantErr {
				t.Errorf("got violation=%v, want=%v", rv != nil, tt.wantErr)
			}
		})
	}
}

func TestCheckBasicRequiresUsername(t *testing.T) {
	tests := []struct {
		name     string
		username types.String
		auth     types.String
		wantErr  bool
	}{
		{"basic with username ok", types.StringValue("bob"), types.StringValue(authTypeBasic), false},
		{"basic with unknown username ok", types.StringUnknown(), types.StringValue(authTypeBasic), false},
		{"basic without username rejected", types.StringNull(), types.StringValue(authTypeBasic), true},
		{"basic with empty username rejected", types.StringValue(""), types.StringValue(authTypeBasic), true},
		{"none without username ok", types.StringNull(), types.StringValue(authTypeNone), false},
		{"auth null skips", types.StringNull(), types.StringNull(), false},
		{"auth unknown skips", types.StringNull(), types.StringUnknown(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rv := checkBasicRequiresUsername(WebhookResourceModel{Username: tt.username, AuthenticationType: tt.auth})
			if (rv != nil) != tt.wantErr {
				t.Errorf("got violation=%v, want=%v", rv != nil, tt.wantErr)
			}
		})
	}
}

func TestCheckHeaderAuthRequiresHeader(t *testing.T) {
	tests := []struct {
		name    string
		header  types.String
		auth    types.String
		wantErr bool
	}{
		{"header-auth with header ok", types.StringValue("{}"), types.StringValue(authTypeHeader), false},
		{"header-auth with unknown header ok", types.StringUnknown(), types.StringValue(authTypeHeader), false},
		{"header-auth without header rejected", types.StringNull(), types.StringValue(authTypeHeader), true},
		{"header-auth with empty header rejected", types.StringValue(""), types.StringValue(authTypeHeader), true},
		{"none without header ok", types.StringNull(), types.StringValue(authTypeNone), false},
		{"auth unknown skips", types.StringNull(), types.StringUnknown(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rv := checkHeaderAuthRequiresHeader(WebhookResourceModel{Header: tt.header, AuthenticationType: tt.auth})
			if (rv != nil) != tt.wantErr {
				t.Errorf("got violation=%v, want=%v", rv != nil, tt.wantErr)
			}
		})
	}
}

func TestCheckSmartGroupIDRequiresSmartEvent(t *testing.T) {
	tests := []struct {
		name    string
		gid     types.Int64
		event   types.String
		wantErr bool
	}{
		{"gid with smart event ok", types.Int64Value(29), types.StringValue("SmartGroupComputerMembershipChange"), false},
		{"gid with mobile smart event ok", types.Int64Value(29), types.StringValue("SmartGroupMobileDeviceMembershipChange"), false},
		{"gid with non-smart event rejected", types.Int64Value(29), types.StringValue("ComputerAdded"), true},
		{"no gid ok", types.Int64Null(), types.StringValue("ComputerAdded"), false},
		{"event unknown skips", types.Int64Value(29), types.StringUnknown(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rv := checkSmartGroupIDRequiresSmartEvent(WebhookResourceModel{SmartGroupID: tt.gid, Event: tt.event})
			if (rv != nil) != tt.wantErr {
				t.Errorf("got violation=%v, want=%v", rv != nil, tt.wantErr)
			}
		})
	}
}
