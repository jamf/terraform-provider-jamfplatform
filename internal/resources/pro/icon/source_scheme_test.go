// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package icon

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// sourceErrors runs the schema validators of one upload-source attribute over
// value and returns the error details. The request carries a configuration in
// which every other attribute is null, which the cross-attribute validators
// some of these attributes also hold need in order to run.
func sourceErrors(t *testing.T, attribute, value string) []string {
	t.Helper()
	var resp resource.SchemaResponse
	NewIconResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	source, ok := resp.Schema.Attributes[attribute].(schema.StringAttribute)
	if !ok {
		t.Fatalf("%s is not a string attribute", attribute)
	}
	ctx := context.Background()
	object, ok := resp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("the schema is not an object")
	}
	raw := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, typ := range object.AttributeTypes {
		raw[name] = tftypes.NewValue(typ, nil)
	}
	raw[attribute] = tftypes.NewValue(tftypes.String, value)
	req := validator.StringRequest{
		Path:           path.Root(attribute),
		PathExpression: path.MatchRoot(attribute),
		Config:         tfsdk.Config{Schema: resp.Schema, Raw: tftypes.NewValue(object, raw)},
		ConfigValue:    types.StringValue(value),
	}
	var out validator.StringResponse
	for _, v := range source.Validators {
		v.ValidateString(ctx, req, &out)
	}
	var details []string
	for _, d := range out.Diagnostics.Errors() {
		details = append(details, d.Detail())
	}
	return details
}

func TestSourceRefusesCloudStorageSchemes(t *testing.T) {
	for _, attribute := range []string{"icon_file_source"} {
		for _, value := range []string{"s3://bucket/file", "azblob://account/container/file"} {
			details := sourceErrors(t, attribute, value)
			if len(details) != 1 || !strings.Contains(details[0], "package_file_source") {
				t.Errorf("%s = %q: want one error naming package_file_source, got %v", attribute, value, details)
			}
		}
		for _, value := range []string{"/tmp/file", "https://example.com/file?sig=abc"} {
			if details := sourceErrors(t, attribute, value); len(details) != 0 {
				t.Errorf("%s = %q: unexpected error %v", attribute, value, details)
			}
		}
	}
}
