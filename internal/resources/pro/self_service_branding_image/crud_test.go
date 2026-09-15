// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_image

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
)

// imageUploadClient returns a Jamf Pro client pointed at a stub server that
// answers the token exchange and accepts one branding image upload, recording
// the bytes it received in uploaded.
//
// The seam is the HTTP boundary rather than an injected interface: the resource
// holds a concrete *pro.Client, and an interface introduced only for a test
// would be a bigger change than the behaviour it pins. The stub is local rather
// than testhelpers.NewMockClient because testhelpers reaches the provider
// package, and the provider registers this one — importing it from an in-package
// test makes that a cycle.
func imageUploadClient(t *testing.T, uploaded *[]byte) *pro.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("upload carried no file part: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Errorf("reading the uploaded file part: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		*uploaded = body

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(pro.BrandingImageURL{
			URL: "https://tenant.example/api/v1/branding-images/download/81",
		})
	}))
	t.Cleanup(server.Close)
	return pro.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// runImageCreate drives Create with the given planned source, the way the
// framework does after ModifyPlan has left every computed value unknown.
func runImageCreate(t *testing.T, r *SelfServiceBrandingImageResource, source string) SelfServiceBrandingImageResourceModel {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diags: %v", schemaResp.Diagnostics)
	}
	nullRaw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)

	planModel := imageTestModel("", source, "", "")
	planModel.ID = types.StringUnknown()
	planModel.SourceHash = types.StringUnknown()
	planModel.URL = types.StringUnknown()

	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: nullRaw}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("plan set: %v", diags)
	}

	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)
	identity := &tfsdk.ResourceIdentity{
		Schema: identityResp.IdentitySchema,
		Raw:    tftypes.NewValue(identityResp.IdentitySchema.Type().TerraformType(ctx), nil),
	}

	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: nullRaw},
		Identity: identity,
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diags: %v", resp.Diagnostics)
	}

	var state SelfServiceBrandingImageResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("created state get: %v", diags)
	}
	return state
}

// TestCreateRecordsTheHashOfTheUploadedBytes is the other half of the issue #373
// fix. Create is the only place the hash is resolved, so it has to be the hash
// of what it sent — not of a second read of the source, which for an unstable
// URL is different bytes.
func TestCreateRecordsTheHashOfTheUploadedBytes(t *testing.T) {
	var served atomic.Int64
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := served.Add(1)
		_, _ = w.Write([]byte(strings.Repeat("x", int(n))))
	}))
	t.Cleanup(sourceServer.Close)

	var uploaded []byte
	r := &SelfServiceBrandingImageResource{client: imageUploadClient(t, &uploaded)}
	state := runImageCreate(t, r, sourceServer.URL+"/banner.png")

	if served.Load() != 1 {
		t.Fatalf("the source was read %d times; Create must read it once so the hash describes the upload", served.Load())
	}
	want := files.ComputeContentSHA256(uploaded)
	if state.SourceHash.ValueString() != want {
		t.Fatalf("source_hash = %q, want %q, the hash of the %d bytes the upload received",
			state.SourceHash.ValueString(), want, len(uploaded))
	}
	if state.ID.ValueString() != "81" {
		t.Fatalf("id = %q, want 81 derived from the upload URL", state.ID.ValueString())
	}
}

// TestCreateFromLocalPathRecordsTheHashOfTheUploadedBytes is the same assertion
// for a local file, where the bytes are stable and the failure would be a
// mismatch between what was hashed and what was sent.
func TestCreateFromLocalPathRecordsTheHashOfTheUploadedBytes(t *testing.T) {
	const content = "local banner bytes"
	source := writeImageFile(t, "banner.png", content)

	var uploaded []byte
	r := &SelfServiceBrandingImageResource{client: imageUploadClient(t, &uploaded)}
	state := runImageCreate(t, r, source)

	if string(uploaded) != content {
		t.Fatalf("the upload received %q, want the file's %q", uploaded, content)
	}
	if want := files.ComputeContentSHA256([]byte(content)); state.SourceHash.ValueString() != want {
		t.Fatalf("source_hash = %q, want %q", state.SourceHash.ValueString(), want)
	}
}
