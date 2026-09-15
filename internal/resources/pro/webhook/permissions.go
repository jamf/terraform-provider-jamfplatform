// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the webhook resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateWebhookByID",
	"GetWebhookByID",
	"UpdateWebhookByID",
	"DeleteWebhookByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the webhook resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the webhook data source calls.
// Lookup is by ID or by exact name, so it needs only the two read endpoints —
// not the resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"GetWebhookByID",
	"GetWebhookByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the webhook data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the webhook list resource calls:
// the list endpoint plus a per-item read used when IncludeResource is set.
var listResourceSDKMethods = []string{
	"ListWebhooks",
	"GetWebhookByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the webhook list resource, appended to its schema Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
