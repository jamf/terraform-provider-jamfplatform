// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_enrollment_profile

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// bigIntStringOrNull converts a nil-safe *BigInt to a TF String (nil/zero → null).
func bigIntStringOrNull(b *proclassic.BigInt) types.String {
	if b == nil {
		return types.StringNull()
	}
	s := b.String()
	if s == "" || s == "0" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// intStringOrNull maps a nil/zero *int epoch to a TF String, else its decimal form.
func intStringOrNull(p *int) types.String {
	if p == nil || *p == 0 {
		return types.StringNull()
	}
	return types.StringValue(strconv.Itoa(*p))
}

// int64ValueOrNull maps a nil *int to a null TF Int64, else its value.
func int64ValueOrNull(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

// flattenAttachments builds the Computed attachments list from the SDK block.
func flattenAttachments(a *proclassic.MobileDeviceEnrollmentProfileAttachments) types.List {
	if a == nil || a.Attachment == nil || len(*a.Attachment) == 0 {
		return types.ListValueMust(attachmentObjectType, []attr.Value{})
	}
	elems := make([]attr.Value, 0, len(*a.Attachment))
	for _, att := range *a.Attachment {
		obj := types.ObjectValueMust(attachmentAttrTypes, map[string]attr.Value{
			"id":       intStringOrNull(att.ID),
			"filename": stringOrNull(att.Filename),
			"uri":      stringOrNull(att.URI),
		})
		elems = append(elems, obj)
	}
	return types.ListValueMust(attachmentObjectType, elems)
}

// stringOrNull is a local nil-safe *string → TF String (empty → null).
func stringOrNull(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}
