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
// use_generic false. The server trims surrounding whitespace first, so a
// padded copy also flips; the exact-match validator on `ppd_path` does not
// catch that form. The path is invalid in every mode: with use_generic=true the
// cross-field validator already forbids any ppd_path, so the validator is
// unconditional.
const bundledGenericPPDPath = "/System/Library/Frameworks/ApplicationServices.framework/Versions/A/Frameworks/PrintCore.framework/Resources/Generic.ppd"
