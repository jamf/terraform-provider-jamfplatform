// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/nullprior"
)

func TestSsoSettingsResource_OptionalBlockLeavesPlanUnknownFromNullPrior(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewSsoSettingsResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	nullprior.AssertOptionalBlockLeavesPlanUnknown(t, resp.Schema)
}

func TestSsoSettingsResource_NothingCollapsesAPlan(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewSsoSettingsResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	nullprior.AssertNoPlanCollapse(t, resp.Schema, ".")
}
