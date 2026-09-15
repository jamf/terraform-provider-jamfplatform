// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the notification resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// r.client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateVolumePurchasingSubscriptionV1",
	"GetVolumePurchasingSubscriptionV1",
	"UpdateVolumePurchasingSubscriptionV1",
	"DeleteVolumePurchasingSubscriptionV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the notification resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the registry-backed SDK methods the data source
// calls. data_source.go also calls ResolveVolumePurchasingSubscriptionV1IDByName,
// but that resolver wrapper is not a registry key — its privilege need (the
// list read) is already covered by GetVolumePurchasingSubscriptionV1's
// volume-purchasing-locations:read privilege, so the rendered table is
// complete with just the registry-backed methods.
var dataSourceSDKMethods = []string{
	"GetVolumePurchasingSubscriptionV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the notification data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls: the
// list fetch plus the per-item hydration GET issued when IncludeResource is set.
var listResourceSDKMethods = []string{
	"ListVolumePurchasingSubscriptionsV1",
	"GetVolumePurchasingSubscriptionV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the notification list resource, appended to its Description.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
