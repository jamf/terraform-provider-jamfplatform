// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// WriteIconFixtures writes two PNGs with different file names and different
// pixels into t.TempDir() and returns their paths, for tests that repoint a
// Self Service icon from one jamfplatform_pro_icon to another.
//
// Both properties matter. Jamf Pro stores an icon's filename from the upload
// and derives its CDN uri from the content hash, and the resources under test
// echo both next to the icon id, so a swap between two files that share a name
// or a hash would not exercise the attribute that changes (issue #468).
func WriteIconFixtures(t *testing.T, suffix string) (a, b string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, c color.RGBA) string {
		img := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := range 64 {
			for x := range 64 {
				img.Set(x, y, c)
			}
		}
		p := filepath.Join(dir, name)
		f, err := os.Create(p) //nolint:gosec // path built from t.TempDir()
		if err != nil {
			t.Fatalf("creating icon fixture %s: %v", name, err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			t.Fatalf("encoding icon fixture %s: %v", name, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("closing icon fixture %s: %v", name, err)
		}
		return p
	}
	return write("tf-acc-icon-a-"+suffix+".png", color.RGBA{R: 200, G: 30, B: 30, A: 255}),
		write("tf-acc-icon-b-"+suffix+".png", color.RGBA{R: 30, G: 30, B: 200, A: 255})
}
