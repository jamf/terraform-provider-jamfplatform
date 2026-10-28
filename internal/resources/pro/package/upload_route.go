// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"io"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jcdsupload"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
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

// uploadClient is everything an upload needs from *pro.Client: the binary
// routes, the verification poll's two calls (which include GetPackageV1) and the
// record write the pre-upload rename makes. *pro.Client satisfies it; the
// interface exists so the upload paths are unit-testable end to end.
type uploadClient interface {
	binaryUploader
	packageVerifier
	UpdatePackageV1(ctx context.Context, id string, request *pro.Package) (*pro.Package, error)
}

// directUpload is the direct route's upload call, held in a variable so a test
// can stand in for the S3 transfer jcdsupload.Upload makes, which needs live
// temporary credentials. Production never reassigns it.
var directUpload = func(ctx context.Context, api jcdsupload.API, fileName string, body io.Reader) error {
	return jcdsupload.Upload(ctx, api, fileName, body)
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
		return directUpload(ctx, client, fileName, body)
	}
	_, err := client.UploadPackageV1(ctx, id, fileName, body)
	return err
}

// renameRecordBeforeUpload writes plan's file_name to the package record ahead of
// a binary upload, and only on the direct route when the name is changing.
//
// The direct route carries no record id: JCDS links the uploaded file to its
// record by name when the inventory refreshes. file_name is mutable, and Update
// otherwise writes the new name in the metadata PUT that follows the poll, so
// uploading under the new name while the record still carries the old one leaves
// a file no record names. The poll then waits out the whole timeout, the rename
// never runs, and the orphan stays in JCDS. Renaming first makes the record
// name the file about to arrive. The API route addresses the record by id and
// needs none of this.
//
// The PUT echoes the record as the server holds it, hashes included, with only
// the name changed. It never sends the local digest: the server does not
// validate a user-supplied hash, so one sent here would let the verification poll
// converge on the first tick while JCDS still serves the old binary. size is
// withheld because the server derives it and blanks it on any PUT.
func renameRecordBeforeUpload(ctx context.Context, client uploadClient, route uploadRoute, plan, state *PackageResourceModel) diag.Diagnostics {
	want := plan.FileName.ValueString()
	if route != uploadViaJCDS || want == "" || want == state.FileName.ValueString() {
		return nil
	}

	id := plan.ID.ValueString()
	current, err := client.GetPackageV1(ctx, id)
	if err != nil {
		return errorDiag("Error reading Jamf Pro package before renaming its file", helpers.APIErrorDetail(err))
	}
	if current.FileName == want {
		return nil
	}

	renamed := *current
	renamed.FileName = want
	renamed.Size = nil
	if _, err := client.UpdatePackageV1(ctx, id, &renamed); err != nil {
		return errorDiag("Error renaming the Jamf Pro package file before upload", helpers.APIErrorDetail(err))
	}
	return nil
}

// reuploadFromFile is Update's disk-staging re-upload: rename the record when
// the direct route needs it (renameRecordBeforeUpload), then upload the
// already-rewound file and poll.
func reuploadFromFile(ctx context.Context, client uploadClient, route uploadRoute, plan, state *PackageResourceModel, fileName string, file io.ReadSeeker, local fileDigests, localSize int64, previous fileDigests) diag.Diagnostics {
	if diags := renameRecordBeforeUpload(ctx, client, route, plan, state); diags.HasError() {
		return diags
	}
	return uploadAndPoll(ctx, client, route, plan.ID.ValueString(), fileName, file, local, localSize, previous, state.Size.ValueString())
}

// reuploadFromURL is Update's streaming-URL re-upload: the same rename, then the
// streamed upload and poll.
func reuploadFromURL(ctx context.Context, client uploadClient, route uploadRoute, plan, state *PackageResourceModel, previous fileDigests) diag.Diagnostics {
	if diags := renameRecordBeforeUpload(ctx, client, route, plan, state); diags.HasError() {
		return diags
	}
	return streamURLUploadAndVerify(ctx, client, route, plan, previous, state.Size.ValueString())
}
