// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the ebook resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateEbookByID",
	"GetEbookByID",
	"UpdateEbookByID",
	"DeleteEbookByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the ebook resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the ebook data source calls (a
// by-ID or by-name lookup). It documents only the privileges the data source
// itself needs, not the resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"GetEbookByID",
	"GetEbookByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the ebook data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the ebook list resource calls:
// the bulk list plus a per-item GET issued when IncludeResource (config
// generation) is requested.
var listResourceSDKMethods = []string{
	"ListEbooks",
	"GetEbookByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the ebook list resource, appended to its Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
