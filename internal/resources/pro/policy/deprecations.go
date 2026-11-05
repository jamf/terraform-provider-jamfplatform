// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

// Schema deprecations of jamfplatform_pro_policy, in one place. Each notice names
// its replacement and the earliest removal date, and carries the greppable marker
// the deprecation sweep reads. On or after the date, remove the attribute and its
// builders, drop it from prior state in UpgradeState, bump the schema Version, and
// delete the notice. See STYLE_GUIDE "Jamf Pro Terraform schema deprecation".
//
// DeprecationMessage reaches an operator through terraform plan and validate only.
// tfplugindocs does not render it, so each notice also prefixes the attribute's
// documentation with a callout built from the same text.

// PRO-DEPRECATED remove-after=2027-02-15 replaced-by=script_assignments — the single-child wrapper scripts.scripts collapses into a flat set; an object cannot become a set under its own name.
const scriptsDeprecation = "Deprecated: use `script_assignments` instead, the same set without the wrapping block. " +
	"The provider will not remove `scripts` before 2027-02-15."

// PRO-DEPRECATED remove-after=2027-02-15 replaced-by=printer_assignments — the single-child wrapper printers.printers collapses into a flat set; an object cannot become a set under its own name.
const printersDeprecation = "Deprecated: use `printer_assignments` instead, the same set without the wrapping block. " +
	"The provider will not remove `printers` before 2027-02-15."

// PRO-DEPRECATED remove-after=2027-02-15 replaced-by=dock_item_assignments — the single-child wrapper dock_items.dock_items collapses into a flat set; an object cannot become a set under its own name.
const dockItemsDeprecation = "Deprecated: use `dock_item_assignments` instead, the same set without the wrapping block. " +
	"The provider will not remove `dock_items` before 2027-02-15."

// PRO-DEPRECATED remove-after=2027-02-15 replaced-by=server_side_limitations — general.date_time_limitations splits into the two blocks the admin UI shows on separate tabs and moves out of general.
const generalDateTimeLimitationsDeprecation = "Deprecated: use the top-level `server_side_limitations` (`activation_date`, `expiration_date`) and `client_side_limitations` (`no_execute_on`, `no_execute_start`, `no_execute_end`) instead, which mirror the admin UI's two tabs. " +
	"`general.date_time_limitations` may be removed on or after 2027-02-15."

// PRO-DEPRECATED remove-after=2027-02-15 replaced-by=general.network_requirements — read-only view of general.network_requirements and scope.limitations.network_segment_ids, which carry every value it reports.
const generalNetworkLimitationsDeprecation = "Deprecated: this read-only block repeats values that already have attributes. Read `general.network_requirements` and " +
	"`scope.limitations.network_segment_ids` instead. The provider will not remove `general.network_limitations` before 2027-02-15."

// PRO-DEPRECATED remove-after=2027-02-15 replaced-by=general.target_drive — read-only view of general.target_drive and packages.distribution_point; force_afp_smb and sus cannot be set from Terraform.
const generalOverrideDefaultSettingsDeprecation = "Deprecated: this read-only block repeats values that already have attributes. Read `general.target_drive` and " +
	"`packages.distribution_point` instead. The provider will not remove `general.override_default_settings` before 2027-02-15."

const generalDateTimeLimitationsDeprecatedDescription = "~> **Deprecated.** " + generalDateTimeLimitationsDeprecation + "\n\n" +
	"Schedule limitations for when the policy may run. Only the user-authored date/time inputs are surfaced. " +
	"The derived epoch and UTC siblings Jamf Pro also stores are deterministic transforms of `activation_date` / `expiration_date`, " +
	"reproducible client-side with Terraform stdlib functions such as `formatdate`."
