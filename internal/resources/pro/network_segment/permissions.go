// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the network segment resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry. The classic /networksegments endpoints are served by the proclassic
// SDK sub-package even though the Terraform resource lives under pro/.
var resourceSDKMethods = []string{
	"CreateNetworkSegmentByID",
	"GetNetworkSegmentByID",
	"UpdateNetworkSegmentByID",
	"DeleteNetworkSegmentByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the network segment resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular network segment data
// source calls (lookup by ID or by name).
var dataSourceSDKMethods = []string{
	"GetNetworkSegmentByID",
	"GetNetworkSegmentByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular network segment data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural network segment
// data source calls.
var pluralDataSourceSDKMethods = []string{
	"ListNetworkSegments",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the plural network segment data source.
var pluralDataSourcePrivileges = permissions.Section(proclassic.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the network segment list resource
// calls.
var listResourceSDKMethods = []string{
	"ListNetworkSegments",
	// The per-item hydration GET the list resource issues when
	// IncludeResource asks for full resource state.
	"GetNetworkSegmentByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the network segment list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
