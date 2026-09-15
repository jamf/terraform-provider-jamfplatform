// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the benchmark resource's CRUD path
// calls directly (via r.client.<Method>) in crud.go. It drives the "Required
// Jamf privileges" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateBenchmark",
	"GetBenchmark",
	"DeleteBenchmark",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the benchmark resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(compliancebenchmarks.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the benchmark data source calls in
// data_source.go. The synthetic ResolveBenchmarkIDByName resolver is not a
// registry entry (it delegates to a list read, already covered by
// GetBenchmark's read privilege), so it is excluded.
var dataSourceSDKMethods = []string{
	"GetBenchmark",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the benchmark data source.
var dataSourcePrivileges = permissions.Section(compliancebenchmarks.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the benchmark list resource
// calls in list_resource.go.
var listResourceSDKMethods = []string{
	"ListBenchmarks",
	"GetBenchmark",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the benchmark list resource.
var listResourcePrivileges = permissions.Section(compliancebenchmarks.Privileges, listResourceSDKMethods...)
