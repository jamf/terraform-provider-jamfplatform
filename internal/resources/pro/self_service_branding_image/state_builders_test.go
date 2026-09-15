// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_image

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestDeriveBrandingImageID(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "standard upload URL",
			url:  "https://nmartin.jamfcloud.com/api/v1/branding-images/download/81",
			want: "81",
		},
		{
			name: "trailing slash tolerated",
			url:  "https://nmartin.jamfcloud.com/api/v1/branding-images/download/42/",
			want: "42",
		},
		{
			name:    "non-numeric tail rejected",
			url:     "https://example.com/branding-images/download/not-a-number",
			wantErr: true,
		},
		{
			name:    "no path segment",
			url:     "noslashes",
			wantErr: true,
		},
		{
			name:    "empty url",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deriveBrandingImageID(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("deriveBrandingImageID(%q) = %q, want error", tt.url, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("deriveBrandingImageID(%q) unexpected error: %v", tt.url, err)
			}
			if got != tt.want {
				t.Errorf("deriveBrandingImageID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestAssignUploadedImage(t *testing.T) {
	var state SelfServiceBrandingImageResourceModel
	resp := &pro.BrandingImageURL{URL: "https://tenant.jamfcloud.com/api/v1/branding-images/download/7"}
	if err := assignUploadedImage(&state, resp); err != nil {
		t.Fatalf("assignUploadedImage unexpected error: %v", err)
	}
	if state.ID.ValueString() != "7" {
		t.Errorf("ID = %q, want 7", state.ID.ValueString())
	}
	if state.URL.ValueString() != resp.URL {
		t.Errorf("URL = %q, want %q", state.URL.ValueString(), resp.URL)
	}

	var bad SelfServiceBrandingImageResourceModel
	if err := assignUploadedImage(&bad, &pro.BrandingImageURL{URL: "garbage"}); err == nil {
		t.Error("assignUploadedImage(garbage URL) expected error, got nil")
	}
}
