// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_distribution_point

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestS3UnsignedConflict(t *testing.T) {
	t.Parallel()

	s3 := types.StringValue("AMAZON_S3")
	tests := []struct {
		name     string
		cdnType  types.String
		signed   types.Bool
		conflict bool
	}{
		{"S3 unsigned is refused", s3, types.BoolValue(false), true},
		{"S3 signed is allowed", s3, types.BoolValue(true), false},
		{"S3 with signed unset is refused", s3, types.BoolNull(), true},
		{"S3 with signed unknown defers", s3, types.BoolUnknown(), false},
		{"unknown cdn_type defers", types.StringUnknown(), types.BoolValue(false), false},
		{"null cdn_type defers", types.StringNull(), types.BoolValue(false), false},
		{"Akamai is not an S3 conflict", types.StringValue("AKAMAI"), types.BoolValue(false), false},
		{"JCDS is not an S3 conflict", types.StringValue("JAMF_CLOUD"), types.BoolValue(false), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := s3UnsignedConflict(tc.cdnType, tc.signed); got != tc.conflict {
				t.Fatalf("conflict = %v, want %v", got, tc.conflict)
			}
		})
	}
}

func TestS3DirectoryConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cdnType  types.String
		conflict bool
	}{
		{"S3 refuses directory", types.StringValue("AMAZON_S3"), true},
		{"Akamai keeps directory", types.StringValue("AKAMAI"), false},
		{"JCDS has no directory field", types.StringValue("JAMF_CLOUD"), false},
		{"unknown cdn_type defers", types.StringUnknown(), false},
		{"null cdn_type defers", types.StringNull(), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := s3DirectoryConflict(tc.cdnType); got != tc.conflict {
				t.Fatalf("conflict = %v, want %v", got, tc.conflict)
			}
		})
	}
}
