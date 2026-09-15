// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_distribution_point

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignCloudDistributionPointResourceModel populates the resource model from an
// SDK GET/POST/PATCH response.
//
// Every field is mapped directly from the server value: this singleton GET/PATCH
// always echoes the full object, and the Optional+Computed fields are Computed
// (so the server is authoritative — there is no user intent to preserve on a
// nil, unlike a plain Optional field). The WriteOnly secrets (password,
// private_key) are deliberately NOT assigned — the framework excludes them from
// state and the API never returns them.
func assignCloudDistributionPointResourceModel(state *CloudDistributionPointResourceModel, s *pro.CloudDistributionPoint) {
	state.CdnType = types.StringValue(s.CdnType)
	state.Master = types.BoolPointerValue(s.Master)
	state.Username = types.StringValue(s.Username)
	state.Directory = types.StringPointerValue(s.Directory)
	state.UploadURL = types.StringPointerValue(s.UploadURL)
	state.DownloadURL = types.StringPointerValue(s.DownloadURL)
	state.CdnURL = types.StringPointerValue(s.CdnURL)
	state.RequireSignedURLs = types.BoolPointerValue(s.RequireSignedUrls)
	state.KeyPairID = types.StringPointerValue(s.KeyPairID)
	state.ExpirationSeconds = int64FromIntPointer(s.ExpirationSeconds, state.ExpirationSeconds)
	state.SecondaryAuthRequired = types.BoolPointerValue(s.SecondaryAuthRequired)
	state.SecondaryAuthTimeToLive = int64FromIntPointer(s.SecondaryAuthTimeToLive, state.SecondaryAuthTimeToLive)

	// Computed status echoes — server-authoritative.
	state.SecondaryAuthStatusCode = int64FromIntPointer(s.SecondaryAuthStatusCode, state.SecondaryAuthStatusCode)
	state.HasConnectionSucceeded = types.BoolValue(s.HasConnectionSucceeded)
	state.Message = types.StringValue(s.Message)
	state.InventoryID = types.StringPointerValue(s.InventoryID)
}

// assignCloudDistributionPointDataSourceModel populates the data source model.
func assignCloudDistributionPointDataSourceModel(state *CloudDistributionPointDataSourceModel, s *pro.CloudDistributionPoint) {
	state.CdnType = types.StringValue(s.CdnType)
	state.Master = types.BoolPointerValue(s.Master)
	state.Username = types.StringValue(s.Username)
	state.Directory = types.StringPointerValue(s.Directory)
	state.UploadURL = types.StringPointerValue(s.UploadURL)
	state.DownloadURL = types.StringPointerValue(s.DownloadURL)
	state.CdnURL = types.StringPointerValue(s.CdnURL)
	state.RequireSignedURLs = types.BoolPointerValue(s.RequireSignedUrls)
	state.KeyPairID = types.StringPointerValue(s.KeyPairID)
	state.ExpirationSeconds = int64FromIntPointer(s.ExpirationSeconds, types.Int64Null())
	state.SecondaryAuthRequired = types.BoolPointerValue(s.SecondaryAuthRequired)
	state.SecondaryAuthTimeToLive = int64FromIntPointer(s.SecondaryAuthTimeToLive, types.Int64Null())
	state.SecondaryAuthStatusCode = int64FromIntPointer(s.SecondaryAuthStatusCode, types.Int64Null())
	state.HasConnectionSucceeded = types.BoolValue(s.HasConnectionSucceeded)
	state.Message = types.StringValue(s.Message)
	state.InventoryID = types.StringPointerValue(s.InventoryID)
}

// int64FromIntPointer converts an SDK *int to a types.Int64, preserving the
// current value when the server omits the field (nil).
func int64FromIntPointer(v *int, current types.Int64) types.Int64 {
	if v == nil {
		return current
	}
	return types.Int64Value(int64(*v))
}
