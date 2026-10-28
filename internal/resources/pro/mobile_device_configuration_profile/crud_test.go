// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

const updateTestPayload = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict>
<key>PayloadType</key><string>Configuration</string>
<key>PayloadIdentifier</key><string>AUTHORED-UUID</string>
<key>PayloadUUID</key><string>AUTHORED-UUID</string>
<key>PayloadVersion</key><integer>1</integer>
</dict></plist>`

type putBodies struct {
	mu     sync.Mutex
	bodies []string
}

func (p *putBodies) add(b string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bodies = append(p.bodies, b)
}

func (p *putBodies) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.bodies...)
}

// runUpdateWithoutStateUUID drives Update with a state whose general.uuid is
// null and a live profile that reports liveUUID (empty means Jamf Pro reports
// none), and returns the response with the PUT bodies the stub saw.
func runUpdateWithoutStateUUID(t *testing.T, liveUUID string) (resource.UpdateResponse, []string) {
	t.Helper()
	ctx := context.Background()
	recorder := &putBodies{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"t","token_type":"Bearer","expires_in":3600}`))
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		switch r.Method {
		case http.MethodGet:
			uuid := ""
			if liveUUID != "" {
				uuid = "<uuid>" + liveUUID + "</uuid>"
			}
			_, _ = w.Write([]byte(`<configuration_profile><general><id>7</id><name>x</name>` + uuid +
				`<payloads>` + html.EscapeString(updateTestPayload) + `</payloads></general></configuration_profile>`))
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("reading the PUT body: %v", err)
			}
			recorder.add(string(body))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`<configuration_profile><id>7</id></configuration_profile>`))
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	t.Cleanup(server.Close)
	client := jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0))
	r := &Resource{client: proclassic.New(client)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	model := ResourceModel{
		ID: types.StringValue("7"),
		General: &GeneralModel{
			Name:     types.StringValue("x"),
			Payloads: types.StringValue(updateTestPayload),
		},
		Timeouts: helpers.NewResourceTimeoutsNullValue(timeoutAttributeTypes),
	}
	seed := tfsdk.State{Schema: schemaResp.Schema}
	if diags := seed.Set(ctx, &model); diags.HasError() {
		t.Fatalf("seeding state and plan: %v", diags)
	}
	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: seed.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: seed.Raw.Copy()},
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: seed.Raw.Copy()},
	}, &resp)
	return resp, recorder.snapshot()
}

// TestUpdate_NullStateUUIDTakesTheLiveUUID pins the fallback: a PUT that omits
// the top-level PayloadUUID makes Jamf Pro mint a new one (wire-probed
// 2026-10-27), which would give the profile a new identity on enrolled
// devices. With no UUID in state, update must send the one Jamf Pro holds, not
// the authored value and not nothing.
func TestUpdate_NullStateUUIDTakesTheLiveUUID(t *testing.T) {
	t.Parallel()
	const live = "LIVE-SERVER-UUID"
	_, puts := runUpdateWithoutStateUUID(t, live)
	if len(puts) != 1 {
		t.Fatalf("expected exactly one PUT, got %d", len(puts))
	}
	if !strings.Contains(puts[0], live) {
		t.Errorf("PUT must carry the live UUID %q; body: %s", live, puts[0])
	}
	if strings.Contains(puts[0], "AUTHORED-UUID") {
		t.Errorf("PUT must not carry the authored top-level UUID; body: %s", puts[0])
	}
}

// TestUpdate_NoUUIDAnywhereRefusesToWrite pins the refusal: with neither state
// nor Jamf Pro holding a UUID, an update would rotate the profile's identity,
// so it must stop before any write.
func TestUpdate_NoUUIDAnywhereRefusesToWrite(t *testing.T) {
	t.Parallel()
	resp, puts := runUpdateWithoutStateUUID(t, "")
	if len(puts) != 0 {
		t.Fatalf("expected no PUT, got %d: %v", len(puts), puts)
	}
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Cannot preserve the profile's identity" {
		t.Errorf("unexpected summary %q", got)
	}
}
