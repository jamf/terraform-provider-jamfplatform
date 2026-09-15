// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package icon

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignIconResourceModel copies the SDK IconResponse fields into state.
func assignIconResourceModel(state *IconResourceModel, resp *pro.IconResponse) {
	state.ID = types.StringValue(strconv.Itoa(resp.ID))
	state.URL = types.StringValue(resp.URL)
}
