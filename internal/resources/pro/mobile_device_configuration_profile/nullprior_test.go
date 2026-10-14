// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/nullprior"
)

func TestResource_OptionalBlockLeavesPlanUnknownFromNullPrior(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	nullprior.AssertOptionalBlockLeavesPlanUnknown(t, resp.Schema)
}
