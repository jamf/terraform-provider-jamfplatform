// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

// Sentinel that the classic /printers endpoint stores when a printer has no
// category assigned. Visible on every GET; the provider decodes it to null
// in state and rejects it as a literal user input to avoid confusion.
const categoryUnassignedSentinel = "No category assigned"

// bundledGenericPPDPath is the `ppd_path` the classic /printers endpoint treats
// as "use the bundled macOS Generic.ppd". Wire-probed 2026-11-04 on POST: sent
// beside use_generic=false, this exact string makes Jamf Pro store
// use_generic=true and keep the rest, while a different path, a different
// case, a longer suffix or the same string in another directory all leave
// use_generic false. The server trims surrounding whitespace first (space,
// newline, tab and CR all probed, same day), so a padded copy also flips.
const bundledGenericPPDPath = "/System/Library/Frameworks/ApplicationServices.framework/Versions/A/Frameworks/PrintCore.framework/Resources/Generic.ppd"
