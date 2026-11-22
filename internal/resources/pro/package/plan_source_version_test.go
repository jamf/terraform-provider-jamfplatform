// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filesources"
)

func TestPlanMarkerVerdict(t *testing.T) {
	cases := []struct {
		name  string
		facts markerFacts
		want  markerVerdict
	}{
		{"local or url source", markerFacts{native: false, examined: true, objectMarker: "etag:a", stateMarker: "etag:a"}, markerNone},
		{"create", markerFacts{native: true, creating: true, examined: true, objectMarker: "etag:a"}, markerReplace},
		{"create, object not examined", markerFacts{native: true, creating: true}, markerDefer},
		{"unchanged", markerFacts{native: true, examined: true, objectMarker: "etag:a", stateMarker: "etag:a"}, markerKeep},
		{"object changed", markerFacts{native: true, examined: true, objectMarker: "etag:b", stateMarker: "etag:a"}, markerReplace},
		{"no marker in state", markerFacts{native: true, examined: true, objectMarker: "etag:a"}, markerReplace},
		{"source string changed to an object with the same etag", markerFacts{native: true, sourceChanged: true, examined: true, objectMarker: "etag:a", stateMarker: "etag:a"}, markerReplace},
		{"not examined", markerFacts{native: true, stateMarker: "etag:a"}, markerDefer},
		{"store reported no marker", markerFacts{native: true, examined: true, stateMarker: "etag:a"}, markerDefer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := planMarkerVerdict(tc.facts); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// sourcePlan describes one ModifyPlan call for planSourceVersion. A nil state
// map is a create. The plan starts from planned, with the marker Unknown, as
// the framework leaves a Computed attribute that has no plan modifier in any
// plan that differs from state.
type sourcePlan struct {
	state   map[string]string
	planned map[string]string
	unknown []string
}

func (p sourcePlan) run(t *testing.T, r *PackageResource) *resource.ModifyPlanResponse {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	objType := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	build := func(values map[string]string, unknown []string) tftypes.Value {
		vals := make(map[string]tftypes.Value, len(objType.AttributeTypes))
		for name, typ := range objType.AttributeTypes {
			vals[name] = tftypes.NewValue(typ, nil)
		}
		for name, v := range values {
			vals[name] = tftypes.NewValue(tftypes.String, v)
		}
		for _, name := range unknown {
			vals[name] = tftypes.NewValue(objType.AttributeTypes[name], tftypes.UnknownValue)
		}
		return tftypes.NewValue(objType, vals)
	}

	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)}
	if p.state != nil {
		state.Raw = build(p.state, nil)
	}
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: build(p.planned, append([]string{"package_file_source_version"}, p.unknown...))}
	resp := &resource.ModifyPlanResponse{Plan: plan}
	r.planSourceVersion(ctx, resource.ModifyPlanRequest{State: state, Plan: plan}, resp)
	return resp
}

func plannedString(t *testing.T, resp *resource.ModifyPlanResponse, attr string) types.String {
	t.Helper()
	var v types.String
	if diags := resp.Plan.GetAttribute(context.Background(), path.Root(attr), &v); diags.HasError() {
		t.Fatalf("read %s: %v", attr, diags)
	}
	return v
}

func recordedPackage(source, marker string) map[string]string {
	values := map[string]string{
		"id": "7", "package_file_source": source,
		"sha3_512": "aa", "sha256": "bb", "md5": "cc", "hash_value": "aa", "size": "13",
	}
	if marker != "" {
		values["package_file_source_version"] = marker
	}
	return values
}

func assertUploadDerived(t *testing.T, resp *resource.ModifyPlanResponse, wantUnknown bool) {
	t.Helper()
	for _, attr := range uploadDerivedAttributes {
		if got := plannedString(t, resp, attr).IsUnknown(); got != wantUnknown {
			t.Errorf("%s unknown = %v, want %v", attr, got, wantUnknown)
		}
	}
}

func TestPlanSourceVersion_UnchangedObjectKeepsTheMarkerAndTheHashes(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	recorded := recordedPackage(objectSource, `etag:"abc"`)

	resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{sources: sources})

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if got := plannedString(t, resp, "package_file_source_version"); got.ValueString() != `etag:"abc"` || got.IsUnknown() {
		t.Errorf("marker = %s, want the value in state", got)
	}
	assertUploadDerived(t, resp, false)
	if heads, gets := store.requests(); heads != 1 || gets != 0 {
		t.Errorf("made %d metadata and %d read requests, want one metadata request", heads, gets)
	}
}

func TestPlanSourceVersion_ChangedObjectPlansTheNewMarkerAsKnown(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	store.replace("newer bytes", `"def"`)
	recorded := recordedPackage(objectSource, `etag:"abc"`)

	resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{sources: sources})

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if got := plannedString(t, resp, "package_file_source_version"); got.IsUnknown() || got.ValueString() != `etag:"def"` {
		t.Errorf("marker = %s, want the new ETag as a known value", got)
	}
	assertUploadDerived(t, resp, true)
	if _, gets := store.requests(); gets != 0 {
		t.Error("a plan must not read the object")
	}
}

func TestPlanSourceVersion_NullMarkerIsADifference(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")
	recorded := recordedPackage(objectSource, "")

	resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{sources: sources})

	if got := plannedString(t, resp, "package_file_source_version"); got.ValueString() != `etag:"abc"` {
		t.Errorf("marker = %s, want the object's ETag", got)
	}
	assertUploadDerived(t, resp, true)
}

func TestPlanSourceVersion_AdoptingASourceAfterImportPlansItsMarker(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")
	imported := map[string]string{"id": "7", "sha3_512": "aa", "sha256": "bb", "md5": "cc", "hash_value": "aa", "size": "13"}

	resp := sourcePlan{state: imported, planned: recordedPackage(objectSource, "")}.run(t, &PackageResource{sources: sources})

	if got := plannedString(t, resp, "package_file_source_version"); got.ValueString() != `etag:"abc"` {
		t.Errorf("marker = %s, want the object's ETag", got)
	}
	assertUploadDerived(t, resp, true)
}

func TestPlanSourceVersion_CreatePlansTheMarker(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")

	resp := sourcePlan{planned: map[string]string{"package_file_source": objectSource}, unknown: uploadDerivedAttributes}.run(t, &PackageResource{sources: sources})

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if got := plannedString(t, resp, "package_file_source_version"); got.ValueString() != `etag:"abc"` {
		t.Errorf("marker = %s, want the object's ETag", got)
	}
}

func TestPlanSourceVersion_OtherSourcesPlanNoMarker(t *testing.T) {
	for name, source := range map[string]string{
		"local path": "/tmp/app.pkg",
		"https url":  "https://example.invalid/app.pkg?sig=abc",
		"none":       "",
	} {
		t.Run(name, func(t *testing.T) {
			recorded := recordedPackage(source, "")
			if source == "" {
				delete(recorded, "package_file_source")
			}
			resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{})
			if resp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if got := plannedString(t, resp, "package_file_source_version"); !got.IsNull() {
				t.Errorf("marker = %s, want null and not (known after apply)", got)
			}
			assertUploadDerived(t, resp, false)
		})
	}
}

func TestPlanSourceVersion_LeavingCloudStorageClearsTheMarker(t *testing.T) {
	resp := sourcePlan{
		state:   recordedPackage(objectSource, `etag:"abc"`),
		planned: recordedPackage("/tmp/app.pkg", `etag:"abc"`),
	}.run(t, &PackageResource{})

	if got := plannedString(t, resp, "package_file_source_version"); !got.IsNull() {
		t.Errorf("marker = %s, want null", got)
	}
}

func TestPlanSourceVersion_UnknownSourceDefers(t *testing.T) {
	resp := sourcePlan{
		state:   recordedPackage(objectSource, `etag:"abc"`),
		planned: recordedPackage(objectSource, `etag:"abc"`),
		unknown: []string{"package_file_source"},
	}.run(t, &PackageResource{})

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if got := plannedString(t, resp, "package_file_source_version"); !got.IsUnknown() {
		t.Errorf("marker = %s, want unknown", got)
	}
}

func TestPlanSourceVersion_VersionedSourceIsNotExaminedOnceRecorded(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	versioned := objectSource + "?versionId=v1"
	recorded := recordedPackage(versioned, `etag:"abc"`)

	resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{sources: sources})

	if got := plannedString(t, resp, "package_file_source_version"); got.ValueString() != `etag:"abc"` {
		t.Errorf("marker = %s", got)
	}
	assertUploadDerived(t, resp, false)
	if heads, _ := store.requests(); heads != 0 {
		t.Errorf("made %d requests for an immutable version already recorded", heads)
	}

	resp = sourcePlan{state: recordedPackage(versioned, ""), planned: recorded}.run(t, &PackageResource{sources: sources})
	if got := plannedString(t, resp, "package_file_source_version"); got.ValueString() != `etag:"abc"` {
		t.Errorf("marker = %s", got)
	}
	if heads, _ := store.requests(); heads != 1 {
		t.Errorf("made %d requests, want one for a version with no marker yet", heads)
	}
}

func TestPlanSourceVersion_MissingObjectDefersWithAWarning(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	store.missing = true
	recorded := recordedPackage(objectSource, `etag:"abc"`)

	resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{sources: sources})

	if resp.Diagnostics.HasError() {
		t.Fatalf("a missing object must not fail the plan: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 || !strings.Contains(resp.Diagnostics[0].Summary(), "not found") {
		t.Fatalf("want one not-found warning, got %v", resp.Diagnostics)
	}
	if got := plannedString(t, resp, "package_file_source_version"); !got.IsUnknown() {
		t.Errorf("marker = %s, want unknown", got)
	}
	assertUploadDerived(t, resp, true)
}

func TestPlanSourceVersion_RefusedAccessIsAnErrorOnTheSource(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	store.refuse = true
	recorded := recordedPackage(objectSource, `etag:"abc"`)

	resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{sources: sources})

	if resp.Diagnostics.ErrorsCount() != 1 {
		t.Fatalf("want one error, got %v", resp.Diagnostics)
	}
	got := resp.Diagnostics[0]
	if got.Summary() != "Cloud storage refused access to the package file" {
		t.Errorf("summary = %q", got.Summary())
	}
	if strings.Contains(got.Detail(), "secret") {
		t.Errorf("the diagnostic must not carry the secret key: %q", got.Detail())
	}
}

func TestPlanSourceVersion_StoreWithNoETagIsAnErrorNotAPerpetualDiff(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	store.omitETag = true
	recorded := recordedPackage(objectSource, `etag:"abc"`)

	resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{sources: sources})

	if resp.Diagnostics.ErrorsCount() != 1 || resp.Diagnostics[0].Summary() != "Cloud storage reports no ETag for the package file" {
		t.Fatalf("want the named error, got %v", resp.Diagnostics)
	}
}

// TestPlanThenUpdate feeds what planSourceVersion really plans into Update,
// so the two halves are held to each other and not to a plan a test wrote by
// hand: whatever the plan showed as known, state must hold after the apply.
func TestPlanThenUpdate(t *testing.T) {
	cases := map[string]struct {
		arrange     func(*objectStore, *jamfStub)
		stateMarker types.String
		wantUploads int
		wantReads   int
	}{
		"unchanged object": {func(*objectStore, *jamfStub) {}, types.StringValue(`etag:"abc"`), 0, 0},
		"changed object": {func(o *objectStore, _ *jamfStub) {
			o.replace("newer package bytes", `"def"`)
		}, types.StringValue(`etag:"abc"`), 1, 1},
		"no marker, bytes already in Jamf Pro": {func(*objectStore, *jamfStub) {}, types.StringNull(), 0, 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sources, store := objectStoreResolver(t, "package bytes")
			stub, client := newJamfStub(t, "package bytes")
			r := &PackageResource{client: client, sources: sources}
			state := recordedModel(stub, types.StringValue(objectSource), tc.stateMarker)
			tc.arrange(store, stub)

			ctx := context.Background()
			var schemaResp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			nullRaw := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)
			prior := tfsdk.State{Schema: schemaResp.Schema, Raw: nullRaw}
			if diags := prior.Set(ctx, &state); diags.HasError() {
				t.Fatalf("state set: %v", diags)
			}
			proposed := state
			proposed.Notes = types.StringValue("after")
			proposed.Size = types.StringUnknown()
			proposed.PackageFileSourceVersion = types.StringUnknown()
			plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: nullRaw}
			if diags := plan.Set(ctx, &proposed); diags.HasError() {
				t.Fatalf("plan set: %v", diags)
			}
			planResp := &resource.ModifyPlanResponse{Plan: plan}
			r.planSourceVersion(ctx, resource.ModifyPlanRequest{State: prior, Plan: plan}, planResp)
			if planResp.Diagnostics.HasError() {
				t.Fatalf("plan diagnostics: %v", planResp.Diagnostics)
			}
			var planned PackageResourceModel
			if diags := planResp.Plan.Get(ctx, &planned); diags.HasError() {
				t.Fatalf("plan get: %v", diags)
			}
			_, readsBefore := store.requests()

			resp, got := runPackageUpdate(t, r, state, planned)

			if resp.Diagnostics.HasError() {
				t.Fatalf("update diagnostics: %v", resp.Diagnostics)
			}
			for attr, pair := range map[string][2]types.String{
				"package_file_source_version": {planned.PackageFileSourceVersion, got.PackageFileSourceVersion},
				"sha3_512":                    {planned.Sha3512, got.Sha3512},
				"sha256":                      {planned.Sha256, got.Sha256},
				"md5":                         {planned.Md5, got.Md5},
				"hash_value":                  {planned.HashValue, got.HashValue},
			} {
				if !pair[0].IsUnknown() && !pair[0].Equal(pair[1]) {
					t.Errorf("%s was planned as %s and applied as %s", attr, pair[0], pair[1])
				}
			}
			if got.PackageFileSourceVersion.IsNull() || got.PackageFileSourceVersion.IsUnknown() {
				t.Errorf("no marker in state after the apply")
			}
			_, readsAfter := store.requests()
			if _, uploads, _ := stub.counts(); uploads != tc.wantUploads || readsAfter-readsBefore != tc.wantReads {
				t.Errorf("uploads=%d reads=%d, want %d and %d", uploads, readsAfter-readsBefore, tc.wantUploads, tc.wantReads)
			}
		})
	}
}

func TestPlanSourceVersion_UnconfiguredProviderIsANamedError(t *testing.T) {
	recorded := recordedPackage(objectSource, `etag:"abc"`)

	resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{})

	if resp.Diagnostics.ErrorsCount() != 1 || resp.Diagnostics[0].Summary() != "Cloud storage source cannot be read" {
		t.Fatalf("want the named error, got %v", resp.Diagnostics)
	}
}

func TestPlanSourceVersion_UnknownProviderConfigurationDefersWithAWarning(t *testing.T) {
	recorded := recordedPackage(objectSource, `etag:"abc"`)
	for name, unresolved := range map[string]string{
		"the whole block":  "file_sources",
		"the s3 block":     "file_sources.s3",
		"a value under s3": "file_sources.s3.assume_role.role_arn",
	} {
		t.Run(name, func(t *testing.T) {
			sources := filesources.New(filesources.Config{}, []string{unresolved})

			resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{sources: sources})

			if resp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if resp.Diagnostics.WarningsCount() != 1 || !strings.Contains(resp.Diagnostics[0].Detail(), unresolved) {
				t.Fatalf("want one warning naming %s, got %v", unresolved, resp.Diagnostics)
			}
			if got := plannedString(t, resp, "package_file_source_version"); !got.IsUnknown() {
				t.Errorf("marker = %s, want unknown", got)
			}
			assertUploadDerived(t, resp, true)
		})
	}
}

func TestPlanSourceVersion_UnknownConfigurationOfAnotherStoreDoesNotDefer(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")
	other := filesources.New(filesources.Config{}, []string{"file_sources.azure.client_id", "file_sources.https[0].bearer_token"})
	if got := other.UnresolvedFor(objectSource); len(got) != 0 {
		t.Fatalf("UnresolvedFor = %v, want none for an S3 source", got)
	}
	recorded := recordedPackage(objectSource, `etag:"abc"`)

	resp := sourcePlan{state: recorded, planned: recorded}.run(t, &PackageResource{sources: sources})

	if resp.Diagnostics.WarningsCount() != 0 {
		t.Errorf("unexpected warning: %v", resp.Diagnostics)
	}
}

func TestPlanSourceVersion_DestroyReadsNothing(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&PackageResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	objType := schemaResp.Schema.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)}
	resp := &resource.ModifyPlanResponse{Plan: plan}

	(&PackageResource{sources: sources}).planSourceVersion(ctx, resource.ModifyPlanRequest{Plan: plan}, resp)

	if heads, _ := store.requests(); heads != 0 || resp.Diagnostics.HasError() {
		t.Errorf("destroy made %d requests, diagnostics %v", heads, resp.Diagnostics)
	}
}
