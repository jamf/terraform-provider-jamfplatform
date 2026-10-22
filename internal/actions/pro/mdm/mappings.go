// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"sort"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// wallpaperLocations maps the `location` value to the wallpaper_setting the
// command carries. The Jamf Pro console labels the three choices Lock Screen,
// Home Screen, and Lock and Home Screens; the wire takes 1, 2 and 3.
//
// The server does not range-check the integer (9 was accepted, SDK v1.9.0 wire
// facts), so this table is the only thing keeping the action inside the three
// documented values.
var wallpaperLocations = map[string]proclassic.MobileDeviceCommandPostGeneralWallpaperSetting{
	"lock_screen":           proclassic.MobileDeviceCommandPostGeneralWallpaperSetting1,
	"home_screen":           proclassic.MobileDeviceCommandPostGeneralWallpaperSetting2,
	"lock_and_home_screens": proclassic.MobileDeviceCommandPostGeneralWallpaperSetting3,
}

// wallpaperLocationNames returns the accepted `location` values in a stable
// order for the validator and the description.
func wallpaperLocationNames() []string {
	names := make([]string, 0, len(wallpaperLocations))
	for name := range wallpaperLocations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
