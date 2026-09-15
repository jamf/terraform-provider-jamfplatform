// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_image

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// deriveBrandingImageID extracts the numeric image ID from the URL returned by
// UploadBrandingImageV1. The upload response carries ONLY a url of the form
// `https://<tenant>/api/v1/branding-images/download/<id>`; the id branding
// configurations reference (`icon_id` / `banner_image_id`) is the final path
// segment. We validate it parses as an integer so a future URL-shape change
// surfaces loudly instead of writing a garbage ID into state.
func deriveBrandingImageID(url string) (string, error) {
	trimmed := strings.TrimRight(url, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx == -1 || idx == len(trimmed)-1 {
		return "", fmt.Errorf("could not derive image ID from upload URL %q", url)
	}
	id := trimmed[idx+1:]
	if _, err := strconv.Atoi(id); err != nil {
		return "", fmt.Errorf("derived image ID %q from upload URL %q is not numeric: %w", id, url, err)
	}
	return id, nil
}

// assignUploadedImage copies the upload response into state: the canonical URL
// and the derived numeric ID.
func assignUploadedImage(state *SelfServiceBrandingImageResourceModel, resp *pro.BrandingImageURL) error {
	id, err := deriveBrandingImageID(resp.URL)
	if err != nil {
		return err
	}
	state.ID = types.StringValue(id)
	state.URL = types.StringValue(resp.URL)
	return nil
}
