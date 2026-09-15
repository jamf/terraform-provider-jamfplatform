// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_code

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// activationCodePut500Warning is the one-shot warning logged when the ProClassic
// PUT /activationcode endpoint returns its known HTTP 500 yet the write is
// confirmed committed via read-back. Tracked upstream in PI-1401.
const activationCodePut500Warning = "Jamf Pro ProClassic PUT /activationcode returned HTTP 500, but a verifying GET confirms the write committed " +
	"(the new code and organization name are reflected on the tenant). This is a known Jamf Pro server bug — the endpoint applies the change " +
	"and then serializes an Internal Server Error response. The provider verified the write via GET and is treating it as successful. Tracked in PI-1401."

// applyActivationCode writes the plan to the ProClassic /activationcode endpoint
// and returns the authoritative server state read back afterwards.
//
// The endpoint has a known server-side bug: the PUT commits the write but
// responds with HTTP 500 (a Tomcat error page, not the documented 201). To stay
// honest we never blindly swallow the error — instead we GET the record and only
// treat the 500 as success when the server reflects exactly the code and
// organization name we sent. A non-500 error, or a 500 whose read-back does not
// match, propagates as a real failure (so an invalid/rejected code still errors).
func (r *ActivationCodeResource) applyActivationCode(ctx context.Context, plan ActivationCodeResourceModel) (*proclassic.ActivationCode, diag.Diagnostics) {
	var diags diag.Diagnostics

	intendedCode := strings.TrimSpace(plan.Code.ValueString())
	intendedOrg := plan.OrganizationName.ValueString()

	//nolint:staticcheck // SA1019: the Classic /activationcode write is deprecated; its successor is not adopted — see deprecation.go.
	writeErr := r.client.UpdateActivationCode(ctx, buildActivationCodeInput(plan))

	//nolint:staticcheck // SA1019: the Classic /activationcode endpoint is deprecated with no replacement read — see deprecation.go.
	got, getErr := r.client.GetActivationCode(ctx)
	if getErr != nil {
		if writeErr != nil {
			diags.AddError("Error setting Jamf Pro activation code", helpers.APIErrorDetail(writeErr))
		} else {
			diags.AddError("Error reading Jamf Pro activation code after write", helpers.APIErrorDetail(getErr))
		}
		return nil, diags
	}

	if writeErr != nil {
		if helpers.IsServerError(writeErr) && activationCodeMatches(got, intendedCode, intendedOrg) {
			tflog.Warn(ctx, activationCodePut500Warning)
		} else {
			diags.AddError("Error setting Jamf Pro activation code", helpers.APIErrorDetail(writeErr))
			return nil, diags
		}
	}

	return got, diags
}

// activationCodeMatches reports whether the server state reflects the code and
// organization name the caller intended to write.
func activationCodeMatches(got *proclassic.ActivationCode, code, org string) bool {
	if got == nil {
		return false
	}
	gotCode := ""
	if got.Code != nil {
		gotCode = *got.Code
	}
	gotOrg := ""
	if got.OrganizationName != nil {
		gotOrg = *got.OrganizationName
	}
	return gotCode == code && gotOrg == org
}
