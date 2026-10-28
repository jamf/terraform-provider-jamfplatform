// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// stubCDPReader answers the one call preflightUploadDestination makes.
type stubCDPReader struct {
	cdp *pro.CloudDistributionPoint
	err error
}

func (s stubCDPReader) GetCloudDistributionPointV1(context.Context) (*pro.CloudDistributionPoint, error) {
	return s.cdp, s.err
}

func TestPreflightUploadDestination(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		reader    stubCDPReader
		wantError bool
		wantRoute uploadRoute
	}{
		{
			name:      "no distribution point refuses the upload",
			reader:    stubCDPReader{cdp: &pro.CloudDistributionPoint{CdnType: pro.CloudDistributionPointCdnTypeNone}},
			wantError: true,
		},
		{
			name:      "absent record refuses the upload",
			reader:    stubCDPReader{cdp: nil},
			wantError: true,
		},
		{
			name:      "jamf cloud allows the upload",
			reader:    stubCDPReader{cdp: &pro.CloudDistributionPoint{CdnType: pro.CloudDistributionPointCdnTypeJamfCloud}},
			wantError: false,
			wantRoute: uploadViaJCDS,
		},
		{
			name:      "amazon s3 uploads through the API",
			reader:    stubCDPReader{cdp: &pro.CloudDistributionPoint{CdnType: pro.CloudDistributionPointCdnTypeAmazonS3}},
			wantError: false,
			wantRoute: uploadViaAPI,
		},
		{
			// Fails open: the check converts a slow failure into a fast one and
			// must not introduce a new one of its own.
			name:      "a failed read does not block the upload and takes the API route",
			reader:    stubCDPReader{err: errors.New("API request failed with status 503")},
			wantError: false,
			wantRoute: uploadViaAPI,
		},
		{
			name:      "akamai uploads through the API",
			reader:    stubCDPReader{cdp: &pro.CloudDistributionPoint{CdnType: pro.CloudDistributionPointCdnTypeAkamai}},
			wantRoute: uploadViaAPI,
		},
		{
			name:      "rackspace uploads through the API",
			reader:    stubCDPReader{cdp: &pro.CloudDistributionPoint{CdnType: pro.CloudDistributionPointCdnTypeRackspaceCloudFiles}},
			wantRoute: uploadViaAPI,
		},
		{
			name:      "a type this provider has not seen uploads through the API",
			reader:    stubCDPReader{cdp: &pro.CloudDistributionPoint{CdnType: "SOMETHING_NEW"}},
			wantRoute: uploadViaAPI,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			route, diags := preflightUploadDestination(context.Background(), tc.reader)
			if got := diags.HasError(); got != tc.wantError {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tc.wantError, diags)
			}
			if !tc.wantError && route != tc.wantRoute {
				t.Errorf("route = %v, want %v", route, tc.wantRoute)
			}
			if !tc.wantError {
				return
			}
			detail := diags.Errors()[0].Detail()
			for _, want := range []string{"package_file_source", "jamfplatform_pro_cloud_distribution_point"} {
				if !strings.Contains(detail, want) {
					t.Errorf("diagnostic detail does not mention %q: %s", want, detail)
				}
			}
		})
	}
}
