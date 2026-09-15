// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tenant_id

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TenantIDDataSource implements the Terraform data source for the Jamf Pro
// tenant identifier.
type TenantIDDataSource struct {
	client *pro.Client
}

// TenantIDDataSourceModel represents the Terraform data source model for the
// Jamf Pro tenant identifier lookup.
type TenantIDDataSourceModel struct {
	ID       types.String   `tfsdk:"id"`
	TenantID types.String   `tfsdk:"tenant_id"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}
