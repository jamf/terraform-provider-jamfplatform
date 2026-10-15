// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func contentPlanFixture(t *testing.T, stateSha3, source string) (resource.ModifyPlanRequest, *resource.ModifyPlanResponse) {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	(&PackageResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	objType := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	build := func(overrides map[string]string) tftypes.Value {
		vals := make(map[string]tftypes.Value, len(objType.AttributeTypes))
		for name, typ := range objType.AttributeTypes {
			vals[name] = tftypes.NewValue(typ, nil)
		}
		for name, v := range overrides {
			vals[name] = tftypes.NewValue(tftypes.String, v)
		}
		return tftypes.NewValue(objType, vals)
	}

	state := tfsdk.State{Schema: schemaResp.Schema, Raw: build(map[string]string{
		"id": "7", "package_file_source": source, "sha3_512": stateSha3, "sha256": "old", "md5": "old", "hash_value": stateSha3, "size": "1",
	})}
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: build(map[string]string{
		"id": "7", "package_file_source": source, "sha3_512": stateSha3, "sha256": "old", "md5": "old", "hash_value": stateSha3, "size": "1",
	})}
	return resource.ModifyPlanRequest{State: state, Plan: plan}, &resource.ModifyPlanResponse{Plan: plan}
}

func writeContentFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.pkg")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func planValueUnknown(t *testing.T, resp *resource.ModifyPlanResponse, attr string) bool {
	t.Helper()
	var v types.String
	if diags := resp.Plan.GetAttribute(context.Background(), path.Root(attr), &v); diags.HasError() {
		t.Fatalf("read %s: %v", attr, diags)
	}
	return v.IsUnknown()
}

var uploadDerivedAttrs = []string{"sha3_512", "sha256", "md5", "hash_value", "size"}

func TestPlanUploadedContentChange_ChangedBytesGoUnknown(t *testing.T) {
	src := writeContentFile(t, "new bytes")
	req, resp := contentPlanFixture(t, "00ff", src)

	planUploadedContentChange(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	for _, attr := range uploadDerivedAttrs {
		if !planValueUnknown(t, resp, attr) {
			t.Errorf("%s should be Unknown when the file's bytes differ from state", attr)
		}
	}
}

func TestPlanUploadedContentChange_UnchangedBytesCarryForward(t *testing.T) {
	src := writeContentFile(t, "same bytes")
	sha, _, err := HashStreamSHA3(mustOpen(t, src))
	if err != nil {
		t.Fatal(err)
	}
	req, resp := contentPlanFixture(t, sha, src)

	planUploadedContentChange(context.Background(), req, resp)

	for _, attr := range uploadDerivedAttrs {
		if planValueUnknown(t, resp, attr) {
			t.Errorf("%s must carry forward when the bytes match state", attr)
		}
	}
}

func TestPlanUploadedContentChange_EmptyStateDigestGoesUnknown(t *testing.T) {
	src := writeContentFile(t, "bytes")
	req, resp := contentPlanFixture(t, "", src)

	planUploadedContentChange(context.Background(), req, resp)

	if !planValueUnknown(t, resp, "sha3_512") {
		t.Error("an imported record with no recorded digest must plan a re-upload")
	}
}

func TestPlanUploadedContentChange_UnreadableOrRemoteSourceIsLeftAlone(t *testing.T) {
	for name, src := range map[string]string{
		"missing file": filepath.Join(t.TempDir(), "absent.pkg"),
		"url":          "https://example.invalid/a.pkg",
	} {
		t.Run(name, func(t *testing.T) {
			req, resp := contentPlanFixture(t, "00ff", src)
			planUploadedContentChange(context.Background(), req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			for _, attr := range uploadDerivedAttrs {
				if planValueUnknown(t, resp, attr) {
					t.Errorf("%s must be untouched", attr)
				}
			}
		})
	}
}

func mustOpen(t *testing.T, p string) *os.File {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
