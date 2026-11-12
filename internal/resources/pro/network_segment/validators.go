// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"context"
	"fmt"
	"net"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// ipv4Address validates that a string attribute is a valid IPv4 address.
// Jamf Pro network segments are IPv4-only; IPv6 inputs are rejected even though
// net.ParseIP would accept them.
func ipv4Address() validator.String { return ipv4Validator{} }

type ipv4Validator struct{}

func (ipv4Validator) Description(context.Context) string {
	return "value must be a valid IPv4 address (e.g. 10.0.0.1)."
}

func (ipv4Validator) MarkdownDescription(context.Context) string {
	return "Value must be a valid IPv4 address (e.g. `10.0.0.1`)."
}

func (ipv4Validator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	v := req.ConfigValue.ValueString()
	if ip := net.ParseIP(v); ip == nil || ip.To4() == nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid IPv4 address",
			fmt.Sprintf("Expected a valid IPv4 address, got %q. Jamf Pro network segments are IPv4-only.", v),
		)
	}
}

// requiredWhenTrue validates that a bool attribute set to true is accompanied
// by a non-null sibling string attribute. false and null impose nothing:
// `terraform query -generate-config-out` writes override_buildings = false for
// a segment with no building, and that configuration must validate.
func requiredWhenTrue(sibling path.Expression) validator.Bool {
	return requiredWhenTrueValidator{sibling: sibling}
}

type requiredWhenTrueValidator struct{ sibling path.Expression }

func (v requiredWhenTrueValidator) Description(context.Context) string {
	return fmt.Sprintf("when true, %s must also be set.", v.sibling)
}

func (v requiredWhenTrueValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v requiredWhenTrueValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || !req.ConfigValue.ValueBool() {
		return
	}
	paths, diags := req.Config.PathMatches(ctx, req.PathExpression.Merge(v.sibling))
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
	for _, p := range paths {
		var val attr.Value
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p, &val)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if val.IsUnknown() || !val.IsNull() {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Attribute Combination",
		fmt.Sprintf("%s must be specified when %s is true.", v.sibling, req.Path),
	)
}
