// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/gatewaystub"
)

// TestIsAcceptedAsyncDelete pins which DELETE replies the ebook Delete switch is
// allowed to read as an accepted async deletion, since taking that branch clears
// the resource from Terraform state.
func TestIsAcceptedAsyncDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			// The gateway's own 404: the request reached no Jamf service, so
			// nothing was accepted. Fails if the !IsGatewayUnrouted term goes.
			name: "gateway unrouted 404",
			err:  &jamfplatform.APIResponseError{StatusCode: 404, Body: "404 page not found"},
			want: false,
		},
		{
			// Same reply as served, with the trailing newline the helper trims.
			name: "gateway unrouted 404 with trailing newline",
			err:  &jamfplatform.APIResponseError{StatusCode: 404, Body: "404 page not found\n"},
			want: false,
		},
		{
			// A refusal the operator has to act on: the integration lacks the
			// delete privilege. Fails if the !IsForbiddenError term goes.
			name: "forbidden",
			err:  &jamfplatform.APIResponseError{StatusCode: 403, Body: `{"httpStatus":403,"errors":[{"code":"BAD_PERMISSIONS"}]}`},
			want: false,
		},
		{
			// The endpoint's genuine accepted-but-misleading reply.
			name: "classic 400 on an accepted async delete",
			err:  &jamfplatform.APIResponseError{StatusCode: 400, Body: "<html><head><title>Status page</title></head><body>Bad Request</body></html>"},
			want: true,
		},
		{
			// Still a client error the branch should accept: a Jamf-shaped 404
			// is a service reply, not the gateway's.
			name: "jamf json 404",
			err:  &jamfplatform.APIResponseError{StatusCode: 404, Body: `{"httpStatus":404,"errors":[{"code":"EBOOK_NOT_FOUND"}]}`},
			want: true,
		},
		{
			name: "server error",
			err:  &jamfplatform.APIResponseError{StatusCode: 500, Body: `{"httpStatus":500}`},
			want: false,
		},
		{
			name: "non-api error",
			err:  errors.New("dial tcp: connection refused"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isAcceptedAsyncDelete(tt.err); got != tt.want {
				t.Errorf("isAcceptedAsyncDelete(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
}

// TestIsAcceptedAsyncDelete_EdgePageIsNotAcceptance covers the one 4xx the table
// above cannot reach. An edge error page is marked inside the SDK's unexported
// transport, so a hand-built *APIResponseError can never carry the marker and a
// table case for it would assert nothing — hence the stub.
//
// CloudFront serving a 404 is a client error that is neither the gateway's
// unrouted reply nor a 403, so before the !IsEdgeBlocked term it read as an
// accepted deletion and dropped a live ebook from state.
func TestIsAcceptedAsyncDelete_EdgePageIsNotAcceptance(t *testing.T) {
	t.Parallel()

	err := gatewaystub.ErrorFrom(t, gatewaystub.Reply{
		Status:      http.StatusNotFound,
		ContentType: "text/html",
		Body:        gatewaystub.CloudFrontPage(gatewaystub.CloudFrontNotFound),
	})

	if isAcceptedAsyncDelete(err) {
		t.Errorf("isAcceptedAsyncDelete = true for a CloudFront 404 page, so Delete clears "+
			"state for an ebook that was never deleted: %v", err)
	}
}
