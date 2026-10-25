// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package site

import (
	"slices"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the site resource's CRUD path calls.
// It mirrors the "SDK endpoints used" block in crud.go and drives the "Required
// Jamf privileges" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateSiteByID",
	"GetSiteByID",
	"UpdateSiteByID",
	"DeleteSiteByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the site resource, appended to its MarkdownDescription.
// collisionSDKMethods lists the SDK methods the plan-time name collision check calls. The
// check lists every existing site once per plan, so the table documents the read it needs.
var collisionSDKMethods = []string{
	"ListSites",
}

var resourcePrivileges = permissions.Section(proclassic.Privileges, slices.Concat(resourceSDKMethods, collisionSDKMethods)...)

// dataSourceSDKMethods lists the SDK methods the singular site data source
// calls — lookup by ID or by name. It documents only the privileges that
// construct needs, not the resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"GetSiteByID",
	"GetSiteByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular site data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural site data source
// calls — a single list fetch, filtered client-side.
var pluralDataSourceSDKMethods = []string{
	"ListSites",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the plural site data source.
var pluralDataSourcePrivileges = permissions.Section(proclassic.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the site list resource calls —
// a single list fetch, filtered client-side.
var listResourceSDKMethods = []string{
	"ListSites",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the site list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
