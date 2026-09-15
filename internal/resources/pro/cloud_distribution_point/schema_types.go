// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_distribution_point

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// cdnTypeNone is the server's sentinel for "no cloud distribution point
// configured". GET returns this (HTTP 200, never 404) once the object is
// deleted; Read treats it as resource-absent and removes the resource from
// state. It is NOT a settable cdn_type — the API rejects it on POST/PATCH.
const cdnTypeNone = pro.CloudDistributionPointCdnTypeNone

// validCdnTypes are the settable cdn_type values, sourced from the server's own
// validation error: "Allowed values are: [RACKSPACE_CLOUD_FILES, AMAZON_S3,
// AKAMAI, JAMF_CLOUD]".
//
// Deliberately narrower than pro.CloudDistributionPointCdnTypeValues(), which
// also generates NONE — the read-only sentinel documented above, which POST and
// PATCH reject. The set is curated; the spellings are the SDK's.
var validCdnTypes = []string{
	pro.CloudDistributionPointCdnTypeJamfCloud,
	pro.CloudDistributionPointCdnTypeAmazonS3,
	pro.CloudDistributionPointCdnTypeAkamai,
	pro.CloudDistributionPointCdnTypeRackspaceCloudFiles,
}

// cloudDistributionPointTimeoutAttributeTypes defines the timeout attribute types.
var cloudDistributionPointTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}
