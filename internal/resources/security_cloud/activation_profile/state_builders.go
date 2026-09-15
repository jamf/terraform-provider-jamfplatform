// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// applyReadState copies what a read can actually tell us onto the model.
//
// The read model is the activation code and nothing else — no name, no
// capabilities, no platforms, no group and no paused state — so this deliberately
// does not touch any configured attribute. Everything else the model already
// holds is what was sent at create time, and it stays authoritative because there
// is nothing to reconcile it against. That is why every configured attribute is
// RequiresReplace: state is the only record of them.
func applyReadState(model *ActivationProfileResourceModel, profile *securitycloud.ActivationProfile) {
	model.ID = types.StringValue(profile.Code)
}
