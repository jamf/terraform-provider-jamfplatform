// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

func TestCollisionSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(pro.Privileges, collisionSDKMethods...); len(missing) > 0 {
		t.Fatalf("collisionSDKMethods not present in pro.Privileges (SDK drift): %v", missing)
	}
}

func TestCollisionSDKMethods_MatchCalls(t *testing.T) {
	src, err := os.ReadFile("collision.go")
	if err != nil {
		t.Fatalf("reading collision.go: %v", err)
	}
	var called []string
	for _, m := range regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`).FindAllStringSubmatch(string(src), -1) {
		if _, ok := pro.Privileges[m[1]]; ok && !slices.Contains(called, m[1]) {
			called = append(called, m[1])
		}
	}
	slices.Sort(called)
	want := slices.Sorted(slices.Values(collisionSDKMethods))
	if !slices.Equal(called, want) {
		t.Fatalf("collision.go calls %v, collisionSDKMethods declares %v", called, want)
	}
}
