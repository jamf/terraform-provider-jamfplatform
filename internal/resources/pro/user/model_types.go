// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
)

// UserDataSource implements the Terraform data source for a single Jamf Pro
// inventory user record (/api/v1/users). Lookup is by ID or by exact username —
// exactly one must be supplied.
type UserDataSource struct {
	client *pro.Client
}

// UserDataSourceModel represents the Terraform data source model for a Jamf Pro
// inventory user lookup.
type UserDataSourceModel struct {
	ID                   types.String   `tfsdk:"id"`
	Username             types.String   `tfsdk:"username"`
	FullName             types.String   `tfsdk:"full_name"`
	EmailAddress         types.String   `tfsdk:"email_address"`
	PhoneNumber          types.String   `tfsdk:"phone_number"`
	Position             types.String   `tfsdk:"position"`
	ManagedAppleID       types.String   `tfsdk:"managed_apple_id"`
	EnableCustomPhotoURL types.Bool     `tfsdk:"enable_custom_photo_url"`
	CustomPhotoURL       types.String   `tfsdk:"custom_photo_url"`
	Timeouts             timeouts.Value `tfsdk:"timeouts"`
}

// UsersDataSourceModel represents the Terraform data source model for inventory
// user searches.
type UsersDataSourceModel struct {
	ID       types.String                 `tfsdk:"id"`
	Users    []UsersDataSourceResultModel `tfsdk:"users"`
	Filters  []filters.FilterModel        `tfsdk:"filter"`
	Timeouts timeouts.Value               `tfsdk:"timeouts"`
}

// UsersDataSourceResultModel represents a single user in the search results.
type UsersDataSourceResultModel struct {
	ID                   types.String `tfsdk:"id"`
	Username             types.String `tfsdk:"username"`
	FullName             types.String `tfsdk:"full_name"`
	EmailAddress         types.String `tfsdk:"email_address"`
	PhoneNumber          types.String `tfsdk:"phone_number"`
	Position             types.String `tfsdk:"position"`
	ManagedAppleID       types.String `tfsdk:"managed_apple_id"`
	EnableCustomPhotoURL types.Bool   `tfsdk:"enable_custom_photo_url"`
	CustomPhotoURL       types.String `tfsdk:"custom_photo_url"`
}
