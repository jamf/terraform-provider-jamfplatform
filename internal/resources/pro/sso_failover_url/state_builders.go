// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_failover_url

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignResourceModel populates the resource model from an SDK response.
func assignResourceModel(state *SsoFailoverURLResourceModel, s *pro.SsoFailoverData) {
	if s == nil {
		return
	}
	if s.FailoverURL != "" {
		state.FailoverURL = types.StringValue(s.FailoverURL)
	} else {
		state.FailoverURL = types.StringNull()
	}
	if s.GenerationTime != 0 {
		state.GenerationTime = types.Int64Value(s.GenerationTime)
		state.GenerationTimeUTC = types.StringValue(formatGenerationTimeUTC(s.GenerationTime))
	} else {
		state.GenerationTime = types.Int64Null()
		state.GenerationTimeUTC = types.StringNull()
	}
}

// assignDataSourceModel populates the data source model from an SDK response.
func assignDataSourceModel(state *SsoFailoverURLDataSourceModel, s *pro.SsoFailoverData) {
	if s == nil {
		return
	}
	if s.FailoverURL != "" {
		state.FailoverURL = types.StringValue(s.FailoverURL)
	} else {
		state.FailoverURL = types.StringNull()
	}
	if s.GenerationTime != 0 {
		state.GenerationTime = types.Int64Value(s.GenerationTime)
		state.GenerationTimeUTC = types.StringValue(formatGenerationTimeUTC(s.GenerationTime))
	} else {
		state.GenerationTime = types.Int64Null()
		state.GenerationTimeUTC = types.StringNull()
	}
}

// formatGenerationTimeUTC converts a Jamf Pro Unix-millisecond timestamp to
// an RFC3339 UTC string.
func formatGenerationTimeUTC(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
