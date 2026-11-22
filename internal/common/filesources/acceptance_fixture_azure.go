// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package filesources

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// PutBlobForAcceptance writes body to an azblob:// source as a block blob, in
// one Put Blob request, replacing whatever is there.
//
// Like PutForAcceptance it exists for the acceptance suite alone and is
// compiled only under the acceptance build tag. It uses the driver's own
// credential, so the identity needs Storage Blob Data Contributor on the
// container where a provider user needs only Storage Blob Data Reader.
func (r *Resolver) PutBlobForAcceptance(ctx context.Context, src string, body []byte) error {
	return r.blobWriteForAcceptance(ctx, http.MethodPut, src, body)
}

// DeleteBlobForAcceptance removes the blob behind an azblob:// source.
func (r *Resolver) DeleteBlobForAcceptance(ctx context.Context, src string) error {
	return r.blobWriteForAcceptance(ctx, http.MethodDelete, src, nil)
}

// blobWriteForAcceptance sends one authorised write for the blob.
func (r *Resolver) blobWriteForAcceptance(ctx context.Context, method, src string, body []byte) error {
	ref, source, err := r.driver(src)
	if err != nil {
		return err
	}
	driver, ok := source.(*azureDriver)
	if !ok {
		return fmt.Errorf("%s is not an Azure Blob Storage source", ref)
	}
	token, suffix, err := driver.credential(ctx)
	if err != nil {
		return err
	}
	bearer, err := token(ctx)
	if err != nil {
		return err
	}
	segments := strings.Split(ref.Blob, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	target := driver.endpoint(ref.Account, suffix) + "/" + url.PathEscape(ref.Container) + "/" + strings.Join(segments, "/")
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("x-ms-version", blobAPIVersion)
	if method == http.MethodPut {
		req.Header.Set("x-ms-blob-type", "BlockBlob")
		req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	resp, err := (&http.Client{Transport: driver.transport}).Do(req)
	if err != nil {
		return fmt.Errorf("writing %s: %w", ref, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("writing %s: Azure Blob Storage answered HTTP %d (%s)", ref, resp.StatusCode, resp.Header.Get("x-ms-error-code"))
	}
	return nil
}
