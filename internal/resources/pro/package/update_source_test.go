// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// jamfStub is a stand-in for the Jamf Pro package endpoints Update calls. It
// holds one package record and counts what was asked of it. An upload replaces
// the record's digests and size with those of the bytes received, at once,
// which is the converged state the verification poll waits for.
//
// The seam is the HTTP boundary, as in the icon resource's tests: the resource
// holds a concrete *pro.Client, and the stub is local because testhelpers
// reaches the provider package, which registers this one.
type jamfStub struct {
	mu                sync.Mutex
	record            pro.Package
	distributionReads int
	uploads           int
	updates           int
	creates           int
}

func (j *jamfStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	j.mu.Lock()
	defer j.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/auth/token":
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
	case strings.HasSuffix(r.URL.Path, "/cloud-distribution-point"):
		j.distributionReads++
		_ = json.NewEncoder(w).Encode(pro.CloudDistributionPoint{CdnType: "AMAZON_S3"})
	case strings.Contains(r.URL.Path, "/refresh-inventory"):
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(r.URL.Path, "/packages/7/upload"):
		j.uploads++
		file, _, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		digests, size, _ := hashStream(file)
		j.record.Sha3512, j.record.Sha256, j.record.Md5 = &digests.sha3, &digests.sha256, &digests.md5
		j.record.HashValue = &digests.sha3
		j.record.Size = new(strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(pro.HrefResponse{ID: "7"})
	case strings.HasSuffix(r.URL.Path, "/packages") && r.Method == http.MethodPost:
		j.creates++
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(pro.HrefResponse{ID: "7"})
	case strings.HasSuffix(r.URL.Path, "/packages/7") && r.Method == http.MethodPut:
		j.updates++
		var sent pro.Package
		_ = json.NewDecoder(r.Body).Decode(&sent)
		j.record.Notes = sent.Notes
		_ = json.NewEncoder(w).Encode(j.record)
	case strings.HasSuffix(r.URL.Path, "/packages/7"):
		_ = json.NewEncoder(w).Encode(j.record)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (j *jamfStub) counts() (distributionReads, uploads, updates int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.distributionReads, j.uploads, j.updates
}

// newJamfStub returns the stub and a client pointed at it. The record holds
// the digests of body, or none when body is empty, the state of a package Jamf
// Pro never hashed.
func newJamfStub(t *testing.T, body string) (*jamfStub, *pro.Client) {
	t.Helper()
	stub := &jamfStub{record: pro.Package{
		ID:                  new("7"),
		PackageName:         "App",
		FileName:            "app.pkg",
		CategoryID:          "-1",
		Priority:            10,
		Size:                new("13"),
		CloudTransferStatus: new(cloudStatusReady),
	}}
	if body != "" {
		digests, _, _ := hashStream(strings.NewReader(body))
		stub.record.Sha3512, stub.record.Sha256, stub.record.Md5 = &digests.sha3, &digests.sha256, &digests.md5
		stub.record.HashValue = &digests.sha3
	}
	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)
	return stub, pro.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// recordedModel is the state of package 7 as a refresh leaves it, with the
// record's digests, the given source and the given marker.
func recordedModel(stub *jamfStub, source, marker types.String) PackageResourceModel {
	text := func(p *string) types.String {
		if p == nil {
			return types.StringValue("")
		}
		return types.StringValue(*p)
	}
	return PackageResourceModel{
		ID:                       types.StringValue("7"),
		DisplayName:              types.StringValue("App"),
		FileName:                 types.StringValue("app.pkg"),
		CategoryID:               types.StringValue("-1"),
		Notes:                    types.StringValue("before"),
		Priority:                 types.Int64Value(10),
		PackageFileSource:        source,
		PackageFileSourceVersion: marker,
		Sha3512:                  text(stub.record.Sha3512),
		Sha256:                   text(stub.record.Sha256),
		Md5:                      text(stub.record.Md5),
		HashValue:                text(stub.record.HashValue),
		Size:                     text(stub.record.Size),
		Timeouts:                 helpers.NewResourceTimeoutsNullValue(packageTimeoutAttributeTypes),
	}
}

// runPackageUpdate drives Update from state to plan and returns the response.
func runPackageUpdate(t *testing.T, r *PackageResource, state, plan PackageResourceModel) (resource.UpdateResponse, PackageResourceModel) {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	nullRaw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)

	planned := tfsdk.Plan{Schema: schemaResp.Schema, Raw: nullRaw}
	if diags := planned.Set(ctx, &plan); diags.HasError() {
		t.Fatalf("plan set: %v", diags)
	}
	prior := tfsdk.State{Schema: schemaResp.Schema, Raw: nullRaw}
	if diags := prior.Set(ctx, &state); diags.HasError() {
		t.Fatalf("state set: %v", diags)
	}
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)
	resp := resource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: nullRaw},
		Identity: &tfsdk.ResourceIdentity{
			Schema: identityResp.IdentitySchema,
			Raw:    tftypes.NewValue(identityResp.IdentitySchema.Type().TerraformType(ctx), nil),
		},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: planned, State: prior}, &resp)

	var result PackageResourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &result); diags.HasError() {
			t.Fatalf("state get: %v", diags)
		}
	}
	return resp, result
}

func withUnknownUploadDerived(m PackageResourceModel) PackageResourceModel {
	m.Sha3512, m.Sha256, m.Md5 = types.StringUnknown(), types.StringUnknown(), types.StringUnknown()
	m.HashValue, m.Size = types.StringUnknown(), types.StringUnknown()
	return m
}

func TestUpdate_UnchangedObjectIsNeitherReadNorUploaded(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	source, marker := types.StringValue(objectSource), types.StringValue(`etag:"abc"`)
	state := recordedModel(stub, source, marker)
	plan := state
	plan.Notes = types.StringValue("after")
	plan.Size = types.StringUnknown()

	resp, got := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if heads, gets := store.requests(); heads != 0 || gets != 0 {
		t.Errorf("the store was asked %d times for metadata and %d times for the object, want neither", heads, gets)
	}
	distributionReads, uploads, updates := stub.counts()
	if distributionReads != 0 || uploads != 0 || updates != 1 {
		t.Errorf("distribution point reads=%d uploads=%d metadata updates=%d, want 0, 0 and 1", distributionReads, uploads, updates)
	}
	if got.PackageFileSourceVersion.ValueString() != `etag:"abc"` || got.Notes.ValueString() != "after" {
		t.Errorf("state marker=%s notes=%s", got.PackageFileSourceVersion, got.Notes)
	}
}

func TestUpdate_AdoptingASourceThatMatchesJamfProRecordsTheMarkerWithoutUploading(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	state := recordedModel(stub, types.StringNull(), types.StringNull())
	plan := withUnknownUploadDerived(recordedModel(stub, types.StringValue(objectSource), types.StringValue(`etag:"abc"`)))

	resp, got := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if _, gets := store.requests(); gets != 1 {
		t.Errorf("the object was read %d times, want once to hash it", gets)
	}
	if _, uploads, _ := stub.counts(); uploads != 0 {
		t.Errorf("uploaded %d times; bytes Jamf Pro already holds must not be sent again", uploads)
	}
	if got.PackageFileSourceVersion.ValueString() != `etag:"abc"` {
		t.Errorf("marker = %s, want the object's", got.PackageFileSourceVersion)
	}
}

func TestUpdate_ChangedObjectIsUploadedAndItsMarkerRecorded(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	store.replace("newer package bytes", `"def"`)
	source := types.StringValue(objectSource)
	state := recordedModel(stub, source, types.StringValue(`etag:"abc"`))
	plan := withUnknownUploadDerived(recordedModel(stub, source, types.StringValue(`etag:"def"`)))

	resp, got := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if _, uploads, _ := stub.counts(); uploads != 1 {
		t.Errorf("uploaded %d times, want once", uploads)
	}
	want, _, _ := hashStream(strings.NewReader("newer package bytes"))
	if got.PackageFileSourceVersion.ValueString() != `etag:"def"` || got.Sha256.ValueString() != want.sha256 {
		t.Errorf("marker=%s sha256=%s", got.PackageFileSourceVersion, got.Sha256)
	}
}

func TestUpdate_RecordWithNoDigestIsUploadedToNotTrustedOnSize(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "")
	source := types.StringValue(objectSource)
	state := recordedModel(stub, source, types.StringNull())
	plan := withUnknownUploadDerived(recordedModel(stub, source, types.StringValue(`etag:"abc"`)))

	resp, got := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if _, uploads, _ := stub.counts(); uploads != 1 {
		t.Errorf("uploaded %d times; a matching size alone must not stand in for a digest", uploads)
	}
	if got.PackageFileSourceVersion.ValueString() != `etag:"abc"` {
		t.Errorf("marker = %s", got.PackageFileSourceVersion)
	}
}

func TestUpdate_ObjectReplacedAfterThePlanFailsBeforeAnythingIsUploaded(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "older bytes")
	source := types.StringValue(objectSource)
	state := recordedModel(stub, source, types.StringValue(`etag:"old"`))
	plan := withUnknownUploadDerived(recordedModel(stub, source, types.StringValue(`etag:"abc"`)))
	store.replace("bytes nobody planned", `"zzz"`)

	resp, _ := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.ErrorsCount() != 1 || resp.Diagnostics[0].Summary() != "Package file changed since the plan" {
		t.Fatalf("want the named error, got %v", resp.Diagnostics)
	}
	if _, uploads, updates := stub.counts(); uploads != 0 || updates != 0 {
		t.Errorf("uploads=%d metadata updates=%d, want nothing written", uploads, updates)
	}
}

func TestUpdate_LocalSourceLeavesTheMarkerNull(t *testing.T) {
	stub, client := newJamfStub(t, "package bytes")
	path := writeContentFile(t, "package bytes")
	state := recordedModel(stub, types.StringValue(path), types.StringNull())
	plan := state
	plan.Notes = types.StringValue("after")
	plan.Size = types.StringUnknown()

	resp, got := runPackageUpdate(t, &PackageResource{client: client}, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if !got.PackageFileSourceVersion.IsNull() {
		t.Errorf("marker = %s, want null for a local source", got.PackageFileSourceVersion)
	}
	if _, uploads, _ := stub.counts(); uploads != 0 {
		t.Errorf("uploaded %d times for unchanged local bytes", uploads)
	}
}

func TestUpdate_KeptMarkerStillHoldsTheChecksumToTheBytes(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	source, marker := types.StringValue(objectSource), types.StringValue(`etag:"abc"`)
	state := recordedModel(stub, source, marker)

	wrong := state
	wrong.PackageFileSourceChecksum = types.StringValue(strings.Repeat("0", 128))
	wrong.Size = types.StringUnknown()
	resp, _ := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, wrong)
	if resp.Diagnostics.ErrorsCount() != 1 || !strings.Contains(resp.Diagnostics[0].Summary(), "package_file_source_checksum") {
		t.Fatalf("a wrong checksum must fail the apply, got %v", resp.Diagnostics)
	}
	if _, gets := store.requests(); gets != 1 {
		t.Errorf("the object was read %d times, want once to check it", gets)
	}

	right := state
	right.PackageFileSourceChecksum = types.StringValue(strings.ToUpper(state.Sha3512.ValueString()))
	right.Size = types.StringUnknown()
	resp, _ = runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, right)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if _, gets := store.requests(); gets != 1 {
		t.Errorf("the object was read again for a checksum Jamf Pro's own digest already satisfies")
	}
}

func TestUpdate_UploadDecisionIsTakenAgainstJamfProNotAgainstStaleState(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	source := types.StringValue(objectSource)
	state := recordedModel(stub, source, types.StringNull())
	plan := withUnknownUploadDerived(recordedModel(stub, source, types.StringValue(`etag:"abc"`)))
	foreign, _, _ := hashStream(strings.NewReader("a file put there by somebody else"))
	stub.record.Sha3512, stub.record.Sha256, stub.record.Md5, stub.record.HashValue = &foreign.sha3, &foreign.sha256, &foreign.md5, &foreign.sha3

	resp, got := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if _, uploads, _ := stub.counts(); uploads != 1 {
		t.Fatalf("uploaded %d times; the object matches state but not the file Jamf Pro holds", uploads)
	}
	want, _, _ := hashStream(strings.NewReader("package bytes"))
	if got.Sha256.ValueString() != want.sha256 || got.PackageFileSourceVersion.ValueString() != `etag:"abc"` {
		t.Errorf("sha256=%s marker=%s, want the object's bytes in Jamf Pro beside its marker", got.Sha256, got.PackageFileSourceVersion)
	}
}

func TestUpdate_KeptMarkerDoesNotSkipWhenJamfProDriftedSinceTheRefresh(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	source, marker := types.StringValue(objectSource), types.StringValue(`etag:"abc"`)
	state := recordedModel(stub, source, marker)
	plan := state
	plan.Notes = types.StringValue("after")
	plan.Size = types.StringUnknown()
	foreign, _, _ := hashStream(strings.NewReader("a file put there by somebody else"))
	stub.record.Sha3512, stub.record.Sha256, stub.record.Md5, stub.record.HashValue = &foreign.sha3, &foreign.sha256, &foreign.md5, &foreign.sha3

	resp, got := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if _, gets := store.requests(); gets != 1 {
		t.Errorf("the object was read %d times, want once", gets)
	}
	if _, uploads, _ := stub.counts(); uploads != 1 {
		t.Errorf("uploaded %d times, want the source's bytes put back", uploads)
	}
	if got.Sha256.ValueString() != state.Sha256.ValueString() {
		t.Errorf("sha256 = %s, want the value the plan carried forward", got.Sha256)
	}
}

// runPackageCreate drives Create from plan and returns the response.
func runPackageCreate(t *testing.T, r *PackageResource, plan PackageResourceModel) (resource.CreateResponse, PackageResourceModel) {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	nullRaw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)
	planned := tfsdk.Plan{Schema: schemaResp.Schema, Raw: nullRaw}
	if diags := planned.Set(ctx, &plan); diags.HasError() {
		t.Fatalf("plan set: %v", diags)
	}
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)
	resp := resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: nullRaw},
		Identity: &tfsdk.ResourceIdentity{
			Schema: identityResp.IdentitySchema,
			Raw:    tftypes.NewValue(identityResp.IdentitySchema.Type().TerraformType(ctx), nil),
		},
	}
	r.Create(ctx, resource.CreateRequest{Plan: planned}, &resp)
	var result PackageResourceModel
	if !resp.Diagnostics.HasError() {
		if diags := resp.State.Get(ctx, &result); diags.HasError() {
			t.Fatalf("state get: %v", diags)
		}
	}
	return resp, result
}

func plannedCreate(marker types.String) PackageResourceModel {
	return withUnknownUploadDerived(PackageResourceModel{
		ID:                       types.StringUnknown(),
		DisplayName:              types.StringValue("App"),
		FileName:                 types.StringValue("app.pkg"),
		PackageFileSource:        types.StringValue(objectSource),
		PackageFileSourceVersion: marker,
		Timeouts:                 helpers.NewResourceTimeoutsNullValue(packageTimeoutAttributeTypes),
	})
}

func TestCreate_ReadsTheObjectPinnedAndRecordsItsMarker(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "")
	stub.record.Size = nil

	resp, got := runPackageCreate(t, &PackageResource{client: client, sources: sources}, plannedCreate(types.StringValue(`etag:"abc"`)))

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	want, _, _ := hashStream(strings.NewReader("package bytes"))
	if got.PackageFileSourceVersion.ValueString() != `etag:"abc"` || got.Sha256.ValueString() != want.sha256 {
		t.Errorf("marker=%s sha256=%s", got.PackageFileSourceVersion, got.Sha256)
	}
}

func TestCreate_DeferredMarkerIsSettledFromTheRead(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "")
	stub.record.Size = nil

	resp, got := runPackageCreate(t, &PackageResource{client: client, sources: sources}, plannedCreate(types.StringUnknown()))

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if got.PackageFileSourceVersion.ValueString() != `etag:"abc"` {
		t.Errorf("marker = %s, want the marker of the bytes read", got.PackageFileSourceVersion)
	}
}

func TestCreate_UnreadableSourceFailsBeforeTheRecordIsCreated(t *testing.T) {
	cases := map[string]struct {
		arrange func(*objectStore)
		marker  types.String
		summary string
	}{
		"missing object":          {func(o *objectStore) { o.missing = true }, types.StringUnknown(), "Package file not found in cloud storage"},
		"refused access":          {func(o *objectStore) { o.refuse = true }, types.StringUnknown(), "Cloud storage refused access to the package file"},
		"replaced since the plan": {func(o *objectStore) { o.replace("other", `"zzz"`) }, types.StringValue(`etag:"abc"`), "Package file changed since the plan"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sources, store := objectStoreResolver(t, "package bytes")
			tc.arrange(store)
			stub, client := newJamfStub(t, "")

			resp, _ := runPackageCreate(t, &PackageResource{client: client, sources: sources}, plannedCreate(tc.marker))

			if resp.Diagnostics.ErrorsCount() != 1 || resp.Diagnostics[0].Summary() != tc.summary {
				t.Fatalf("want %q, got %v", tc.summary, resp.Diagnostics)
			}
			stub.mu.Lock()
			defer stub.mu.Unlock()
			if stub.creates != 0 {
				t.Errorf("created %d record(s); a source that cannot be read must leave nothing behind", stub.creates)
			}
		})
	}
}

// runPackageRead drives Read over state and returns the response.
func runPackageRead(t *testing.T, r *PackageResource, state PackageResourceModel) (resource.ReadResponse, PackageResourceModel) {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	prior := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)}
	if diags := prior.Set(ctx, &state); diags.HasError() {
		t.Fatalf("state set: %v", diags)
	}
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)
	resp := resource.ReadResponse{
		State: prior,
		Identity: &tfsdk.ResourceIdentity{
			Schema: identityResp.IdentitySchema,
			Raw:    tftypes.NewValue(identityResp.IdentitySchema.Type().TerraformType(ctx), nil),
		},
	}
	r.Read(ctx, resource.ReadRequest{State: prior}, &resp)
	var result PackageResourceModel
	if diags := resp.State.Get(ctx, &result); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	return resp, result
}

func TestRead_ClearsTheMarkerWhenJamfProHoldsAnotherFile(t *testing.T) {
	stub, client := newJamfStub(t, "package bytes")
	state := recordedModel(stub, types.StringValue(objectSource), types.StringValue(`etag:"abc"`))

	resp, got := runPackageRead(t, &PackageResource{client: client}, state)
	if resp.Diagnostics.WarningsCount() != 0 || got.PackageFileSourceVersion.ValueString() != `etag:"abc"` {
		t.Fatalf("an unchanged record must keep its marker: marker=%s diagnostics=%v", got.PackageFileSourceVersion, resp.Diagnostics)
	}

	foreign, _, _ := hashStream(strings.NewReader("a file put there by somebody else"))
	stub.record.Sha3512, stub.record.Sha256, stub.record.Md5, stub.record.HashValue = &foreign.sha3, &foreign.sha256, &foreign.md5, &foreign.sha3
	resp, got = runPackageRead(t, &PackageResource{client: client}, state)
	if !got.PackageFileSourceVersion.IsNull() {
		t.Errorf("marker = %s, want it cleared", got.PackageFileSourceVersion)
	}
	if resp.Diagnostics.WarningsCount() != 1 || resp.Diagnostics[0].Summary() != "Package file changed outside Terraform" {
		t.Errorf("want the drift warning, got %v", resp.Diagnostics)
	}
	if got.Sha256.ValueString() != foreign.sha256 {
		t.Errorf("state must show the digest Jamf Pro reports")
	}

	local := recordedModel(stub, types.StringValue("/tmp/app.pkg"), types.StringNull())
	local.Sha256 = types.StringValue("an older digest")
	resp, _ = runPackageRead(t, &PackageResource{client: client}, local)
	if resp.Diagnostics.WarningsCount() != 0 {
		t.Errorf("a package with no marker has nothing to clear and must not warn: %v", resp.Diagnostics)
	}
}

func TestUpdate_StreamedPackageWithAnUnchangedObjectSendsNothing(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	state := recordedModel(stub, types.StringValue(objectSource), types.StringValue(`etag:"abc"`))
	state.StreamURLDirectly = types.BoolValue(true)
	plan := state
	plan.Notes = types.StringValue("after")
	plan.Size = types.StringUnknown()

	resp, got := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if heads, gets := store.requests(); heads != 0 || gets != 0 {
		t.Errorf("the store was asked %d times for metadata and %d times for the object, want neither", heads, gets)
	}
	if _, uploads, updates := stub.counts(); uploads != 0 || updates != 1 {
		t.Errorf("uploads=%d metadata updates=%d, want 0 and 1: a streamed package must not be sent again for a metadata edit", uploads, updates)
	}
	if got.PackageFileSourceVersion.ValueString() != `etag:"abc"` || got.Notes.ValueString() != "after" {
		t.Errorf("state marker=%s notes=%s", got.PackageFileSourceVersion, got.Notes)
	}
}

func TestUpdate_StreamedPackageWithAChangedObjectIsStreamedPinned(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	store.replace("newer package bytes", `"def"`)
	source := types.StringValue(objectSource)
	state := recordedModel(stub, source, types.StringValue(`etag:"abc"`))
	state.StreamURLDirectly = types.BoolValue(true)
	plan := withUnknownUploadDerived(recordedModel(stub, source, types.StringValue(`etag:"def"`)))
	plan.StreamURLDirectly = types.BoolValue(true)

	resp, got := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if _, uploads, _ := stub.counts(); uploads != 1 {
		t.Errorf("uploaded %d times, want once", uploads)
	}
	want, _, _ := hashStream(strings.NewReader("newer package bytes"))
	if got.PackageFileSourceVersion.ValueString() != `etag:"def"` || got.Sha256.ValueString() != want.sha256 {
		t.Errorf("marker=%s sha256=%s", got.PackageFileSourceVersion, got.Sha256)
	}

	store.replace("bytes nobody planned", `"zzz"`)
	stale := withUnknownUploadDerived(recordedModel(stub, source, types.StringValue(`etag:"def"`)))
	stale.StreamURLDirectly = types.BoolValue(true)
	replaced := recordedModel(stub, source, types.StringValue(`etag:"old"`))
	replaced.StreamURLDirectly = types.BoolValue(true)
	resp, _ = runPackageUpdate(t, &PackageResource{client: client, sources: sources}, replaced, stale)
	if resp.Diagnostics.ErrorsCount() != 1 || resp.Diagnostics[0].Summary() != "Package file changed since the plan" {
		t.Fatalf("a stream pinned to a replaced object must fail by name, got %v", resp.Diagnostics)
	}
	if _, uploads, _ := stub.counts(); uploads != 1 {
		t.Errorf("uploads=%d; nothing may be sent for an object the plan did not describe", uploads)
	}
}

func TestCreate_StreamedFromCloudStorageRecordsTheMarker(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "")
	stub.record.Size = nil
	plan := plannedCreate(types.StringValue(`etag:"abc"`))
	plan.StreamURLDirectly = types.BoolValue(true)

	resp, got := runPackageCreate(t, &PackageResource{client: client, sources: sources}, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	want, _, _ := hashStream(strings.NewReader("package bytes"))
	if got.PackageFileSourceVersion.ValueString() != `etag:"abc"` || got.Sha256.ValueString() != want.sha256 {
		t.Errorf("marker=%s sha256=%s", got.PackageFileSourceVersion, got.Sha256)
	}
}

func TestUpdate_StreamingRefusesAChecksumItCannotCheck(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	source := types.StringValue(objectSource)
	state := recordedModel(stub, source, types.StringValue(`etag:"abc"`))
	plan := withUnknownUploadDerived(recordedModel(stub, source, types.StringValue(`etag:"abc"`)))
	plan.StreamURLDirectly = types.BoolValue(true)
	plan.PackageFileSourceChecksum = types.StringValue(strings.Repeat("0", 128))

	resp, _ := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.ErrorsCount() != 1 || !strings.Contains(resp.Diagnostics[0].Summary(), "package_file_source_checksum") {
		t.Fatalf("a checksum that cannot be checked must stop the apply, got %v", resp.Diagnostics)
	}
	if _, gets := store.requests(); gets != 0 {
		t.Errorf("the object was read %d times before the refusal", gets)
	}
	if _, uploads, _ := stub.counts(); uploads != 0 {
		t.Errorf("uploaded %d times", uploads)
	}
}

func TestUpdate_StreamedAdoptionHashesTheObjectAndSkipsAnUploadJamfProDoesNotNeed(t *testing.T) {
	cases := map[string]struct {
		jamfHolds   string
		heldSize    string
		stateMarker types.String
		wantReads   int
		wantUploads int
	}{
		"same bytes already in Jamf Pro":                         {"package bytes", "", types.StringNull(), 1, 0},
		"other bytes in Jamf Pro":                                {"older bytes", "", types.StringNull(), 2, 1},
		"Jamf Pro has no digest to compare with":                 {"", "", types.StringNull(), 1, 1},
		"marker changed, bytes rewritten unchanged":              {"package bytes", "", types.StringValue(`etag:"old"`), 1, 0},
		"marker changed, same size, other bytes":                 {"package BYTES", "", types.StringValue(`etag:"old"`), 2, 1},
		"marker changed, Jamf Pro's file is another size":        {"older bytes", "11", types.StringValue(`etag:"old"`), 1, 1},
		"marker changed, Jamf Pro reports no size to compare by": {"package bytes", "-", types.StringValue(`etag:"old"`), 1, 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sources, store := objectStoreResolver(t, "package bytes")
			stub, client := newJamfStub(t, tc.jamfHolds)
			switch tc.heldSize {
			case "":
			case "-":
				stub.record.Size = nil
			default:
				stub.record.Size = new(tc.heldSize)
			}
			source := types.StringValue(objectSource)
			state := recordedModel(stub, source, tc.stateMarker)
			state.StreamURLDirectly = types.BoolValue(true)
			plan := withUnknownUploadDerived(recordedModel(stub, source, types.StringValue(`etag:"abc"`)))
			plan.StreamURLDirectly = types.BoolValue(true)

			resp, got := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

			if resp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			_, reads := store.requests()
			_, uploads, _ := stub.counts()
			if reads != tc.wantReads || uploads != tc.wantUploads {
				t.Errorf("reads=%d uploads=%d, want %d and %d", reads, uploads, tc.wantReads, tc.wantUploads)
			}
			want, _, _ := hashStream(strings.NewReader("package bytes"))
			if got.PackageFileSourceVersion.ValueString() != `etag:"abc"` || got.Sha256.ValueString() != want.sha256 {
				t.Errorf("marker=%s sha256=%s, want the object's marker beside the object's digest", got.PackageFileSourceVersion, got.Sha256)
			}
		})
	}
}

func TestUpdate_StreamedAdoptionOfAReplacedObjectFailsBeforeAnythingIsSent(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	stub, client := newJamfStub(t, "package bytes")
	source := types.StringValue(objectSource)
	state := recordedModel(stub, source, types.StringNull())
	plan := withUnknownUploadDerived(recordedModel(stub, source, types.StringValue(`etag:"abc"`)))
	plan.StreamURLDirectly = types.BoolValue(true)
	store.replace("bytes nobody planned", `"zzz"`)

	resp, _ := runPackageUpdate(t, &PackageResource{client: client, sources: sources}, state, plan)

	if resp.Diagnostics.ErrorsCount() != 1 || resp.Diagnostics[0].Summary() != "Package file changed since the plan" {
		t.Fatalf("want the named error, got %v", resp.Diagnostics)
	}
	if _, uploads, updates := stub.counts(); uploads != 0 || updates != 0 {
		t.Errorf("uploads=%d metadata updates=%d, want nothing written", uploads, updates)
	}
}
