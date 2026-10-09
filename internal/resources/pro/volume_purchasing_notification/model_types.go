// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// VolumePurchasingNotificationResourceModel is the Terraform resource model for a
// Volume Purchasing notification — the Jamf Pro admin UI "Notifications" tab under
// Settings → Volume purchasing. The endpoint and SDK call the feature
// "subscriptions"; "notification" is the UI-aligned vocabulary used throughout.
//
// Wire mapping notes (UI label ← wire name):
//   - name                 → name                       (UI "Display name")
//   - enabled              → enabled                    (UI "Enabled")
//   - triggers             → triggers[]                 (UI trigger checkboxes)
//   - location_ids         → locationIds[]              (UI "Included locations")
//   - internal_recipients  → internalRecipients[].accountId
//   - external_recipients  → externalRecipients[]       (UI "External Recipients")
//   - site_id              → siteId                     (UI "Site"; -1 = none)
//
// The endpoint is FULL-REPLACE on update (wire-probed: a PUT omitting a collection
// resets it to empty, and omitting `enabled` resets it to the server default true).
// So every collection and `enabled` is emitted from state on every write, and an
// empty set clears the field. All four collections are Optional+Computed and
// flatten an empty wire array to an empty set (never null) — null and [] both mean
// "none" under full-replace, so state always mirrors the server echo.
//
// internal_recipients is a flat Set[String] of Jamf Pro account ids. The wire
// element also carries a per-recipient `frequency`, but the only accepted value is
// "DAILY" (the UI exposes no frequency control — the Scope tab is labelled "Users
// to distribute a daily summary email to"), so the provider synthesises
// frequency:"DAILY" on write and drops it on read.
type VolumePurchasingNotificationResourceModel struct {
	ID                 types.String           `tfsdk:"id"`
	Name               types.String           `tfsdk:"name"`
	Enabled            types.Bool             `tfsdk:"enabled"`
	Triggers           types.Set              `tfsdk:"triggers"`
	LocationIDs        types.Set              `tfsdk:"location_ids"`
	InternalRecipients types.Set              `tfsdk:"internal_recipients"`
	ExternalRecipients types.Set              `tfsdk:"external_recipients"`
	SiteID             types.String           `tfsdk:"site_id"`
	Timeouts           resourceTimeouts.Value `tfsdk:"timeouts"`
}

// ExternalRecipientModel models one external recipient row (UI "External
// Recipients": Full Name + Email Address). Both fields are required by the server.
type ExternalRecipientModel struct {
	Email types.String `tfsdk:"email"`
	Name  types.String `tfsdk:"name"`
}

// VolumePurchasingNotificationDataSourceModel is the singular data source model. It
// projects the notification by id or by exact name. Collections are read-only
// Lists per the data-source convention.
type VolumePurchasingNotificationDataSourceModel struct {
	ID                 types.String             `tfsdk:"id"`
	Name               types.String             `tfsdk:"name"`
	Enabled            types.Bool               `tfsdk:"enabled"`
	Triggers           types.List               `tfsdk:"triggers"`
	LocationIDs        types.List               `tfsdk:"location_ids"`
	InternalRecipients types.List               `tfsdk:"internal_recipients"`
	ExternalRecipients types.List               `tfsdk:"external_recipients"`
	SiteID             types.String             `tfsdk:"site_id"`
	Timeouts           datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// volumePurchasingNotificationIdentityModel represents the identity object for the
// resource and list results.
type volumePurchasingNotificationIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// VolumePurchasingNotificationListResourceModel represents the config model for
// list queries. The endpoint has no server-side name filter, so the shared
// client-side substring block is applied locally.
type VolumePurchasingNotificationListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
