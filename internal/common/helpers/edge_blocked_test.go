// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"net/http"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/gatewaystub"
)

// TestIsNotFoundError_EdgePageIsNotGone pins the reason IsEdgeBlocked exists: a
// 404 an edge served is not evidence the object is gone, and reading it as gone
// removes a live object from state on the next refresh. The classic template in
// the same table is what stops the fix from being a blunt "HTML 404 is never
// gone", which would break every /proclassic Read.
func TestIsNotFoundError_EdgePageIsNotGone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		reply    gatewaystub.Reply
		edge     bool
		notFound bool
	}{
		{
			name: "cloudfront 404 page did not reach Jamf",
			reply: gatewaystub.Reply{
				Status: http.StatusNotFound, ContentType: "text/html", Body: gatewaystub.CloudFrontPage(gatewaystub.CloudFrontNotFound),
			},
			edge:     true,
			notFound: false,
		},
		{
			name: "classic status page is Jamf reporting a real not-found",
			reply: gatewaystub.Reply{
				Status: http.StatusNotFound, ContentType: "text/html", Body: gatewaystub.ClassicStatusPage,
			},
			edge:     false,
			notFound: true,
		},
		{
			name: "pro json 404 is Jamf reporting a real not-found",
			reply: gatewaystub.Reply{
				Status: http.StatusNotFound, ContentType: "application/json",
				Body: `{"httpStatus":404,"errors":[{"code":"NOT_FOUND","description":"no such building"}]}`,
			},
			edge:     false,
			notFound: true,
		},
		{
			name: "gateway unrouted 404 is neither",
			reply: gatewaystub.Reply{
				Status: http.StatusNotFound, ContentType: "text/plain; charset=utf-8", Body: "404 page not found\n",
			},
			edge:     false,
			notFound: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := gatewaystub.ErrorFrom(t, tc.reply)

			if got := IsEdgeBlocked(err); got != tc.edge {
				t.Errorf("IsEdgeBlocked = %v, want %v", got, tc.edge)
			}
			if got := IsNotFoundError(err); got != tc.notFound {
				t.Errorf("IsNotFoundError = %v, want %v — reading an edge page as "+
					"\"gone\" deletes a live object from state, and refusing a real "+
					"Jamf 404 leaves a deleted object in it", got, tc.notFound)
			}
		})
	}
}

// TestIsEdgeBlocked_KeepsTheAPIErrorReachable pins the property the fix rests
// on: the SDK wraps rather than replaces, so marking an error as an edge page
// must not cost the status, body or trace id the diagnostics read. If the SDK
// ever wraps without %w this fails here rather than as a nil dereference in one
// of the twenty-odd resource helpers that call AsAPIError.
//
// Driven with the 403 block page rather than a CloudFront 5xx deliberately. The
// assertion is about the wrapper and holds for any marked status, and a 5xx is
// one the SDK retries — which spent fifteen seconds of `make test` proving
// something this does not test.
func TestIsEdgeBlocked_KeepsTheAPIErrorReachable(t *testing.T) {
	t.Parallel()

	err := gatewaystub.ErrorFrom(t, gatewaystub.Reply{
		Status: http.StatusForbidden, ContentType: "text/html", Body: gatewaystub.NginxBlockPage,
	})

	if !IsEdgeBlocked(err) {
		t.Fatalf("IsEdgeBlocked = false for an nginx 403 block page: %v", err)
	}

	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		t.Fatalf("AsAPIError = nil, so the *APIResponseError left the error chain: %v", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusForbidden)
	}
	if IsNotFoundError(err) {
		t.Error("IsNotFoundError = true for a 403")
	}
}
