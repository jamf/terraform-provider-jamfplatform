// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
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

func TestPreflightUploadDestinationWarnsWhenItFallsBack(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		err            error
		wantPermission bool
	}{
		"refused names the read permission": {&jamfplatform.APIResponseError{StatusCode: http.StatusForbidden, Body: `{"httpStatus":403}`}, true},
		"other failure names no permission": {errors.New("API request failed with status 503"), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			route, diags := preflightUploadDestination(context.Background(), stubCDPReader{err: tc.err})
			if route != uploadViaAPI || diags.HasError() || diags.WarningsCount() != 1 {
				t.Fatalf("route=%v diags=%v, want the API route and exactly one warning", route, diags)
			}
			warning := diags.Warnings()[0]
			if warning.Summary() != cdpUnreadableSummary {
				t.Errorf("summary = %q", warning.Summary())
			}
			if got := strings.Contains(warning.Detail(), "Cloud Distribution Point → Read"); got != tc.wantPermission {
				t.Errorf("detail names the read permission = %v, want %v: %s", got, tc.wantPermission, warning.Detail())
			}
			if !strings.Contains(warning.Detail(), "Jamf Cloud Distribution Service files → Create") {
				t.Errorf("detail does not name the upload permission: %s", warning.Detail())
			}
		})
	}
}

func TestUploadFailureDetailNamesTheJCDSPermissionOnARefusal(t *testing.T) {
	t.Parallel()

	refused := fmt.Errorf("jcdsupload.Upload(%q): initiate: %w", "app.pkg", &jamfplatform.APIResponseError{StatusCode: http.StatusForbidden, Body: `{"httpStatus":403}`})
	const permission = "Jamf Cloud Distribution Service files → Create"

	cases := map[string]struct {
		route uploadRoute
		err   error
		want  bool
	}{
		"direct route refused":    {uploadViaJCDS, refused, true},
		"API route refused":       {uploadViaAPI, refused, false},
		"direct route, not a 403": {uploadViaJCDS, errors.New("connection reset"), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			detail := uploadFailureDetail(tc.route, tc.err)
			if got := strings.Contains(detail, permission); got != tc.want {
				t.Errorf("names the permission = %v, want %v: %s", got, tc.want, detail)
			}
			if !strings.Contains(detail, tc.err.Error()) {
				t.Errorf("detail dropped the cause: %s", detail)
			}
		})
	}
}
