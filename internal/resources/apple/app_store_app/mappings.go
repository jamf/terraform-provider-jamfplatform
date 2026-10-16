// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"slices"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/appstore"
)

// defaultReadTimeout bounds one lookup, including the wait for a concurrency slot.
const defaultReadTimeout = 30 * time.Second

// Icon size and format icon_url is filled at. Fixed rather than configurable: it feeds
// jamfplatform_pro_icon, and Self Service has no use for a second size.
const (
	iconSize   = 512
	iconFormat = "png"
)

// targetMac and targetMobileDevice are the target values, named after the Jamf Pro sidebar
// sections ("Mac Apps", "Mobile Device Apps") and the resources the outputs feed.
const (
	targetMac          = "mac"
	targetMobileDevice = "mobile_device"
)

// targetPlatforms maps each target to the SDK platform it asks the App Store for. Mac must ask for
// the Mac flavour: a universal app's Mac listing has its own icon (shaped, with transparency),
// description and version, and an iPhone-only app must be not found rather than returned.
var targetPlatforms = map[string]appstore.Platform{
	targetMac:          appstore.PlatformMac,
	targetMobileDevice: appstore.PlatformMobile,
}

// iconURL picks the image icon_url publishes, filled at iconSize and iconFormat.
//
// A listing carries two: artwork, which on iPhone and iPad is the raw full-bleed square (opaque,
// no corner mask) and on Mac is the shaped icon; and iconArtwork, the same art masked to Apple's
// rounded square on iPhone and iPad, matching artwork on Mac, and absent from some Mac listings.
// artwork is used for both targets, with no fallback, because it is the image Jamf Pro pulls in
// by default when it adds an App Store app itself, so an app record built from this data source
// shows the same icon as one an administrator added in Jamf Pro (maintainer decision 2026-10-16).
func iconURL(app *appstore.App) (string, error) {
	return app.ArtworkURL(iconSize, iconSize, iconFormat)
}

// targetValues returns the accepted target values in a stable order, for the validator and the
// documented list alike.
func targetValues() []string {
	values := make([]string, 0, len(targetPlatforms))
	for label := range targetPlatforms {
		values = append(values, label)
	}
	slices.Sort(values)
	return values
}
