// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package disk_encryption_configuration

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the disk encryption configuration
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateDiskEncryptionConfigurationByID",
	"GetDiskEncryptionConfigurationByID",
	"UpdateDiskEncryptionConfigurationByID",
	"DeleteDiskEncryptionConfigurationByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the disk encryption configuration resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the disk encryption configuration
// data source calls (lookup by ID or by name).
var dataSourceSDKMethods = []string{
	"GetDiskEncryptionConfigurationByID",
	"GetDiskEncryptionConfigurationByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the disk encryption
// configuration list resource calls. The list endpoint returns id+name per
// row; when include_resource is set the list path follows up with a per-item
// GET.
var listResourceSDKMethods = []string{
	"ListDiskEncryptionConfigurations",
	"GetDiskEncryptionConfigurationByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the list resource, appended to its top-level Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
