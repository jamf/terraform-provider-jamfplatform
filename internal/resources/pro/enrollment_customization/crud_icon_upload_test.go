// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package enrollment_customization

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

// iconUploadStub records what an Update did with the icon: the bytes any upload
// received, and how many uploads happened.
type iconUploadStub struct {
	uploaded []byte
	uploads  atomic.Int64
	storedID string
}

// iconUploadClient returns a Jamf Pro client pointed at a stub server carrying
// the four calls an Update makes: the token exchange, the image upload, the
// parent PUT, and the parent GET plus panel list that refreshState issues.
//
// The seam is the HTTP boundary rather than an injected interface: the resource
// holds a concrete *pro.Client, and an interface introduced only for a test
// would be a bigger change than the behaviour it pins. The stub is local rather
// than testhelpers.NewMockClient because testhelpers reaches the provider
// package, and the provider registers this one — importing it from an in-package
// test makes that a cycle.
func iconUploadClient(t *testing.T, stub *iconUploadStub) *pro.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/auth/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})

		case strings.HasSuffix(r.URL.Path, "/enrollment-customizations/images"):
			stub.uploads.Add(1)
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
			stub.uploaded = body
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(pro.BrandingImageURL{
				URL: "https://tenant.example/api/v2/enrollment-customizations/images/9",
			})

		case strings.HasSuffix(r.URL.Path, "/all"):
			_ = json.NewEncoder(w).Encode(pro.EnrollmentCustomizationPanelList{})

		default:
			id := stub.storedID
			_ = json.NewEncoder(w).Encode(pro.EnrollmentCustomizationV2{
				ID:          &id,
				DisplayName: "tf-acc-icon-plan",
				Description: "icon plan modifier coverage",
				SiteID:      "-1",
				EnrollmentCustomizationBrandingSettings: pro.EnrollmentCustomizationBrandingSettings{
					TextColor:       "333333",
					ButtonColor:     "0066cc",
					ButtonTextColor: "ffffff",
					BackgroundColor: "ffffff",
					IconURL:         "https://tenant.example/api/v2/enrollment-customizations/images/9",
				},
			})
		}
	}))
	t.Cleanup(server.Close)
	return pro.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// runIconUpdate drives Update with the given plan and state, the way the
// framework does after ModifyPlan has decided whether the icon needs uploading.
func runIconUpdate(t *testing.T, r *EnrollmentCustomizationResource, stateModel, planModel *EnrollmentCustomizationResourceModel) EnrollmentCustomizationResourceModel {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diags: %v", schemaResp.Diagnostics)
	}
	nullRaw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)

	state := tfsdk.State{Schema: schemaResp.Schema, Raw: nullRaw}
	if diags := state.Set(ctx, stateModel); diags.HasError() {
		t.Fatalf("state set: %v", diags)
	}
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: nullRaw}
	if diags := plan.Set(ctx, planModel); diags.HasError() {
		t.Fatalf("plan set: %v", diags)
	}

	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)
	identity := &tfsdk.ResourceIdentity{
		Schema: identityResp.IdentitySchema,
		Raw:    tftypes.NewValue(identityResp.IdentitySchema.Type().TerraformType(ctx), nil),
	}

	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: nullRaw},
		Identity: identity,
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diags: %v", resp.Diagnostics)
	}

	var got EnrollmentCustomizationResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("updated state get: %v", diags)
	}
	return got
}

// TestUpdateUploadsOnUnknownHashAndRecordsWhatItSent is the other half of the
// issue #373 fix here. An unknown icon_source_hash is ModifyPlan's request to
// upload, and the hash committed afterwards has to be the hash of the bytes the
// upload received — not of a second read of the source, which for an unstable
// URL is different bytes.
func TestUpdateUploadsOnUnknownHashAndRecordsWhatItSent(t *testing.T) {
	var served atomic.Int64
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := served.Add(1)
		_, _ = w.Write([]byte(strings.Repeat("x", int(n))))
	}))
	t.Cleanup(sourceServer.Close)

	stub := &iconUploadStub{storedID: "7"}
	r := &EnrollmentCustomizationResource{client: iconUploadClient(t, stub)}

	source := sourceServer.URL + "/icon.png"
	stateModel := iconPlanTestModel("7", source, files.ComputeContentSHA256([]byte("old bytes")), "https://tenant.example/api/v2/enrollment-customizations/images/3")
	planModel := iconPlanTestModel("7", source+"?v=2", "", "")
	planModel.IconSourceHash = types.StringUnknown()
	planModel.BrandingSettings.IconURL = types.StringUnknown()

	got := runIconUpdate(t, r, &stateModel, &planModel)

	if n := stub.uploads.Load(); n != 1 {
		t.Fatalf("the icon was uploaded %d times, want 1", n)
	}
	if n := served.Load(); n != 1 {
		t.Fatalf("the source was read %d times; Update must read it once so the hash describes the upload", n)
	}
	if want := files.ComputeContentSHA256(stub.uploaded); got.IconSourceHash.ValueString() != want {
		t.Fatalf("icon_source_hash = %q, want %q, the hash of the %d bytes the upload received",
			got.IconSourceHash.ValueString(), want, len(stub.uploaded))
	}
	if got.BrandingSettings.IconURL.ValueString() != "https://tenant.example/api/v2/enrollment-customizations/images/9" {
		t.Fatalf("icon_url = %q, want the URL the upload returned", got.BrandingSettings.IconURL.ValueString())
	}
}

// TestUpdateUploadsNothingOnAKnownHash is the regression this whole shape most
// invites: a hash carried forward from state must leave the icon alone, or every
// apply that renames a customization re-uploads its icon.
func TestUpdateUploadsNothingOnAKnownHash(t *testing.T) {
	stub := &iconUploadStub{storedID: "7"}
	r := &EnrollmentCustomizationResource{client: iconUploadClient(t, stub)}

	stored := files.ComputeContentSHA256([]byte("unchanged bytes"))
	const iconURL = "https://tenant.example/api/v2/enrollment-customizations/images/9"

	stateModel := iconPlanTestModel("7", "./icon.png", stored, iconURL)
	planModel := iconPlanTestModel("7", "./icon.png", stored, iconURL)
	planModel.DisplayName = types.StringValue("tf-acc-icon-plan-renamed")

	got := runIconUpdate(t, r, &stateModel, &planModel)

	if n := stub.uploads.Load(); n != 0 {
		t.Fatalf("the icon was uploaded %d times for a rename, want 0", n)
	}
	if got.IconSourceHash.ValueString() != stored {
		t.Fatalf("icon_source_hash = %q, want the stored %q left alone", got.IconSourceHash.ValueString(), stored)
	}
}
