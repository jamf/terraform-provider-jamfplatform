// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filesources"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// openPackageSource resolves package_file_source into a seekable file for the
// disk-staging upload path. It returns the file, the file name to fall back
// to, the store's marker for the bytes in the file, and a cleanup the caller
// must run.
//
// A local path and an http(s) URL go to files.OpenUploadSource, as they always
// have, and have no marker. The one URL that does not is an https URL whose
// host has a credential in the provider's file_sources.https list: the
// resolver downloads it, sending the credential to that host alone. It has no
// marker either. An s3:// or azblob:// source is copied to a
// temporary file by the provider's cloud storage resolver, with the
// credentials the file_sources block resolves to. A nil resolver refuses such
// a source with a named error, so it never falls through to the local-path
// branch and fails as a missing file.
//
// pin is the marker the plan recorded for a cloud storage object, or empty
// when the plan could not examine it. A pinned read fails with
// filesources.ErrChanged when the object has been replaced since. After a
// pinned read the marker returned is the pin itself, not the store's echo of
// it: the read succeeding is the store's word that they name the same bytes,
// and a store that spells an ETag differently on its two responses must not
// make state disagree with a marker the plan showed as known.
//
// manifest_file_source does not come through here. It reads a local path or a
// URL only, and its schema validator turns the cloud schemes away.
func openPackageSource(ctx context.Context, sources *filesources.Resolver, src, pin string) (*os.File, string, string, func(), error) {
	if sources.Authenticated(src) {
		staged, cleanup, err := sources.StageURL(ctx, src, files.DefaultMaxBytes)
		if err != nil {
			return nil, "", "", cleanup, err
		}
		return staged.File, staged.Filename, "", cleanup, nil
	}
	if !filesources.Native(src) {
		file, filename, cleanup, err := files.OpenUploadSource(ctx, src, files.DefaultMaxBytes)
		return file, filename, "", cleanup, err
	}
	staged, cleanup, err := sources.Stage(ctx, src, pin, files.DefaultMaxBytes)
	if err != nil {
		return nil, "", "", cleanup, err
	}
	if pin != "" {
		return staged.File, staged.Filename, pin, cleanup, nil
	}
	return staged.File, staged.Filename, staged.Marker, cleanup, nil
}

// preflightSource examines a cloud storage source before Create makes the
// package record, and refuses what the read that follows would refuse: an
// object that is missing, out of reach, over the size limit, or no longer the
// one the plan recorded.
//
// Create makes the record first, because the upload needs its ID, and a source
// that then fails to open leaves a record with no state behind it. The next
// apply collides with that record's name. A plan that could not examine the
// object defers to the apply, so a mistyped object name arrives here; one
// metadata request before the record exists turns it into a clean failure. It
// is the same reasoning as preflightUploadDestination. A local path or a URL
// is not examined: that path is unchanged.
func preflightSource(ctx context.Context, sources *filesources.Resolver, src, pin string) error {
	if !filesources.Native(src) {
		return nil
	}
	attributes, err := sources.Stat(ctx, src)
	if err != nil {
		return err
	}
	if pin != "" && attributes.Marker != pin {
		return fmt.Errorf("%w: %s", filesources.ErrChanged, files.RedactSource(src))
	}
	if attributes.Size > files.DefaultMaxBytes {
		return fmt.Errorf("%w: %d bytes, over the %d byte limit", filesources.ErrTooLarge, attributes.Size, files.DefaultMaxBytes)
	}
	return nil
}

// openPackageURLStream opens a source for the streaming upload path and
// returns its body, the file name to fall back to, the declared size, and the
// store's marker for the bytes in the body. The caller closes the body.
//
// A cloud storage object is opened by the resolver, pinned as openPackageSource
// pins it, and resumes by itself if the connection is cut; it always declares
// a size. An https URL whose host has a credential is opened by the resolver
// too. Any other URL goes to files.OpenURLStream, as before. A URL has no
// marker and may declare no size, which is -1.
func openPackageURLStream(ctx context.Context, sources *filesources.Resolver, src, pin string) (io.ReadCloser, string, int64, string, error) {
	switch {
	case filesources.Native(src):
		opened, err := sources.Open(ctx, src, pin, files.DefaultMaxBytes)
		if err != nil {
			return nil, "", -1, "", err
		}
		marker := opened.Marker
		if pin != "" {
			marker = pin
		}
		return opened.Body, opened.Filename, opened.Size, marker, nil
	case sources.Authenticated(src):
		opened, err := sources.OpenURL(ctx, src, files.DefaultMaxBytes)
		if err != nil {
			return nil, "", -1, "", err
		}
		return opened.Body, opened.Filename, opened.Size, "", nil
	}
	body, filename, size, err := files.OpenURLStream(ctx, src, files.DefaultMaxBytes)
	return body, filename, size, "", err
}

// adoptingSource reports whether an Update is giving the package a cloud
// storage source that state holds no marker for: the source was just added or
// changed, or its marker was cleared because Jamf Pro's file changed outside
// Terraform. It is the one case where the object's bytes may well be the ones
// Jamf Pro already holds although no marker says so.
func adoptingSource(plan, state *PackageResourceModel) bool {
	return state.PackageFileSourceVersion.IsNull() || !plan.PackageFileSource.Equal(state.PackageFileSource)
}

// sourceAlreadyHeld reads a cloud storage object through the hashers alone,
// storing nothing and uploading nothing, and reports whether Jamf Pro already
// holds those bytes. It returns the marker of the bytes it read.
//
// It is the streaming path's answer to adoption. That path sends the file
// before it has a digest to compare, so without this an imported package
// given a source with stream_url_directly set would be uploaded in full
// although nothing had changed, and the only way round it was to apply once
// with streaming off, which needs the local disk that streaming exists to
// avoid. Reading the object once to hash it costs a download and no disk, and
// on a match the upload is skipped. On a mismatch the object is read a second
// time to send it, which is why this runs only when adopting: for an object
// whose marker changed, different bytes are the likely answer and the first
// read would be wasted.
//
// The read is pinned like any other, so the bytes hashed are the ones the
// plan described, and it is held to the declared size, so a short read cannot
// pass for a match. held must carry a digest; the caller does not ask when
// Jamf Pro has none, because a size alone proves nothing (sameAsRecorded).
func sourceAlreadyHeld(ctx context.Context, sources *filesources.Resolver, src, pin string, held fileDigests) (string, bool, error) {
	opened, err := sources.Open(ctx, src, pin, files.DefaultMaxBytes)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = opened.Body.Close() }()
	local, read, err := hashStream(opened.Body)
	if err != nil {
		return "", false, err
	}
	if read != opened.Size {
		return "", false, fmt.Errorf("%w: the store declared %d bytes and sent %d", filesources.ErrIncomplete, opened.Size, read)
	}
	marker := opened.Marker
	if pin != "" {
		marker = pin
	}
	return marker, held.matches(local), nil
}

// streamingChecksumProblem is the diagnostic for package_file_source_checksum
// set together with stream_url_directly. The schema refuses the pair, but only
// when both values are known while the configuration is validated; one derived
// from another resource gets past it, and the streaming path would then send
// the file with the checksum never looked at.
func streamingChecksumProblem(plan *PackageResourceModel) diag.Diagnostics {
	if !isConfiguredString(plan.PackageFileSourceChecksum) {
		return nil
	}
	return errorDiag("package_file_source_checksum cannot be checked when streaming",
		"stream_url_directly sends the file before its checksum is known, so package_file_source_checksum cannot be honoured. Remove one of the two.")
}

// plannedPin returns the marker a plan recorded for the object, to pin the
// apply's read to. Empty when the plan held none: the source is not in cloud
// storage, or the plan deferred to the apply.
func plannedPin(plan *PackageResourceModel) string {
	if plan.PackageFileSourceVersion.IsNull() || plan.PackageFileSourceVersion.IsUnknown() {
		return ""
	}
	return plan.PackageFileSourceVersion.ValueString()
}

// settleSourceVersion gives package_file_source_version its value for state
// once an apply has dealt with the source. recorded is the marker of the bytes
// the apply read, and replaces whatever was planned. With nothing read, a
// planned value stands and an Unknown one becomes null: the only way to reach
// state with no read is a source that is not in cloud storage.
func settleSourceVersion(plan *PackageResourceModel, recorded string) {
	switch {
	case recorded != "":
		plan.PackageFileSourceVersion = types.StringValue(recorded)
	case plan.PackageFileSourceVersion.IsUnknown():
		plan.PackageFileSourceVersion = types.StringNull()
	}
}

// sourceUnchanged reports whether an Update can skip reading the source: it is
// a cloud storage object, the source string is the one in state, and the plan
// kept the marker state holds. That is the plan saying the object is the one
// whose bytes Jamf Pro already has, so there is nothing to download, hash or
// upload, and the update is metadata only.
//
// A package_file_source_checksum is honoured without the download when it
// equals the SHA-3-512 Jamf Pro recorded for those bytes. Any other value,
// including one set on a record Jamf Pro never hashed, takes the read, so a
// wrong checksum fails the apply as it does for every other source.
//
// Update adds one condition this cannot see: Jamf Pro's file must still be the
// one state describes (driftedOutsideTerraform against a fresh read).
func sourceUnchanged(plan, state *PackageResourceModel) bool {
	if !isConfiguredString(plan.PackageFileSource) || !filesources.Native(plan.PackageFileSource.ValueString()) {
		return false
	}
	if isConfiguredString(plan.PackageFileSourceChecksum) &&
		!strings.EqualFold(plan.PackageFileSourceChecksum.ValueString(), state.Sha3512.ValueString()) {
		return false
	}
	pin := plannedPin(plan)
	return pin != "" &&
		plan.PackageFileSource.Equal(state.PackageFileSource) &&
		!state.PackageFileSourceVersion.IsNull() &&
		pin == state.PackageFileSourceVersion.ValueString()
}

// sameAsRecorded reports whether the record in Jamf Pro already holds the
// bytes just read, so the upload can be skipped.
//
// For a local path or a URL this is fileDigests.sameContent, which falls back
// to comparing sizes when the record has no digest. For a cloud storage
// source that fallback is refused: skipping the upload there also records the
// object's marker, and a matching byte count proves nothing about the bytes.
// Recording a marker on that evidence would stop the provider ever correcting
// a file that only happens to be the same size. So a record with no digest is
// uploaded to, once, and the marker then describes bytes the provider sent.
func sameAsRecorded(native bool, recorded, local fileDigests, recordedSize string, localSize int64) bool {
	if native {
		return recorded.matches(local)
	}
	return recorded.sameContent(local, recordedSize, localSize)
}

// sourceProblem names a failure to read a cloud storage source, for a
// diagnostic. The kinds a user can act on each get their own summary; the
// rest keep the general one.
func sourceProblem(err error) (summary, detail string) {
	detail = helpers.APIErrorDetail(err)
	switch {
	case errors.Is(err, filesources.ErrChanged):
		return "Package file changed since the plan",
			"The object behind package_file_source was replaced after Terraform planned this change, so the plan no longer describes it, and the apply stopped without sending the replacement. Run the plan again."
	case errors.Is(err, filesources.ErrCredentials):
		return "Cannot authenticate to cloud storage", detail
	case errors.Is(err, filesources.ErrAccessDenied):
		return "Cloud storage refused access to the package file", detail
	case errors.Is(err, filesources.ErrNotFound):
		return "Package file not found in cloud storage", detail
	case errors.Is(err, filesources.ErrUnauthorized):
		return "Server refused the package file download", detail
	case errors.Is(err, filesources.ErrTooLarge):
		return "Package file is over the upload size limit", detail
	case errors.Is(err, filesources.ErrNotConfigured), errors.Is(err, filesources.ErrNoDriver), errors.Is(err, filesources.ErrNoSize):
		return "Cloud storage source cannot be read", detail
	}
	return openProblemSummary, detail
}

// openProblemSummary is the summary for a source failure with no name of its
// own, the one the resource has always used.
const openProblemSummary = "Error opening package binary"

// driftedOutsideTerraform reports whether the file Jamf Pro holds is no longer
// the one a marker was recorded for, by comparing what state recorded with
// what Jamf Pro reports now.
//
// A marker describes the object in cloud storage and says nothing about Jamf
// Pro's copy, so a file replaced in Jamf Pro has to invalidate it, or an
// unchanged object would stop the provider ever putting the right file back.
//
// Only a positive difference counts. Jamf Pro fills digests in after an
// upload, never computes one for some large packages, and blanks size on any
// metadata update until the next inventory refresh, so a value that is empty
// on either side is not evidence of anything. Every algorithm both sides hold
// a digest for is compared, and one that differs is drift. With no algorithm
// in common, the sizes are compared when both are present.
func driftedOutsideTerraform(recorded, current fileDigests, recordedSize, currentSize string) bool {
	was, now := recorded.pairs(), current.pairs()
	compared := false
	for i := range was {
		if was[i].value == "" || now[i].value == "" {
			continue
		}
		if !strings.EqualFold(was[i].value, now[i].value) {
			return true
		}
		compared = true
	}
	if compared {
		return false
	}
	return recordedSize != "" && currentSize != "" && recordedSize != currentSize
}
