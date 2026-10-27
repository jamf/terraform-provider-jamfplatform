// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"io"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jcdsupload"
)

// uploadRoute is how a package binary reaches the tenant's cloud distribution
// point. The route is a property of the distribution point, not of the file.
type uploadRoute int

const (
	// uploadViaAPI sends the binary through UploadPackageV1, which hands it to
	// whichever distribution point the tenant has configured. It is the only
	// route that reaches a third-party store (Amazon S3, Akamai, Rackspace).
	uploadViaAPI uploadRoute = iota

	// uploadViaJCDS sends the binary straight to the Jamf Cloud Distribution
	// Service's storage with the temporary credentials Jamf Pro issues, through
	// jcdsupload.Upload. It is a JCDS-only route: against any other
	// distribution point the file lands in JCDS storage and the package record
	// never links to it, leaving an orphan there and nothing on the distribution
	// point the tenant actually configured.
	uploadViaJCDS
)

func (r uploadRoute) String() string {
	if r == uploadViaJCDS {
		return "jcds"
	}
	return "api"
}

// routeForCdnType picks the upload route for a distribution point type.
//
// Only JAMF_CLOUD takes the direct route, and only on a positive match. Every
// other value, including one this provider has never seen, takes the API route
// because that route reaches the configured distribution point whatever it is,
// where the direct route reaches JCDS and nothing else. NONE never gets here:
// preflightUploadDestination refuses it before any bytes move.
func routeForCdnType(cdnType string) uploadRoute {
	if strings.EqualFold(cdnType, pro.CloudDistributionPointCdnTypeJamfCloud) {
		return uploadViaJCDS
	}
	return uploadViaAPI
}

// binaryUploader is the part of *pro.Client that moving a binary needs: the
// API route's own call plus the two the direct route makes. *pro.Client
// satisfies it; the interface exists so the routing is unit-testable.
type binaryUploader interface {
	jcdsupload.API
	UploadPackageV1(ctx context.Context, id string, fileFilename string, file io.Reader) (*pro.HrefResponse, error)
}

// uploadBinary moves body to the distribution point by route. It only moves
// bytes: the caller still polls the package record until the distribution
// point has registered the file.
//
// The direct route needs no record id because the file is matched to its record
// by name when the inventory refreshes, which is why the record's file_name
// (Required) and fileName here are the same string.
func uploadBinary(ctx context.Context, client binaryUploader, route uploadRoute, id, fileName string, body io.Reader) error {
	if route == uploadViaJCDS {
		return jcdsupload.Upload(ctx, client, fileName, body)
	}
	_, err := client.UploadPackageV1(ctx, id, fileName, body)
	return err
}
