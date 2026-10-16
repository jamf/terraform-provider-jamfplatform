// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/jamfplatform-go-sdk/appstore"
)

// storeURLValidator rejects a store_url the provider cannot read an app id from, at plan time
// rather than on read. It defers on an unknown or null value.
type storeURLValidator struct{}

// Description implements validator.Describer.
func (storeURLValidator) Description(_ context.Context) string {
	return "must be an App Store app link ending in id followed by the app's number"
}

// MarkdownDescription implements validator.Describer.
func (v storeURLValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (storeURLValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := appstore.ParseStoreURL(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid App Store link", storeURLDetail(req.ConfigValue.ValueString()))
	}
}
