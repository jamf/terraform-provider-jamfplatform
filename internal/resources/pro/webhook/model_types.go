// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// WebhookResourceModel is the Terraform resource model for a Jamf Pro webhook
// (the classic /webhooks endpoint). The wire envelope is flat — every field is
// top-level, there is no <general> wrapper — plus one Computed-only
// display_fields reflection. See WEBHOOK_SPIKE.md for the full wire-probe.
//
//   - `Password` is `WriteOnly`: the plaintext (BASIC password or
//     HASH_SIGNATURE signing secret) is sent on writes but never persisted in
//     state. `PasswordWoVersion` is the rotation trigger — bump it to force the
//     next Update to re-send the current `Password`.
//   - `Header` (HEADER auth, JSON) is Sensitive but state-tracked: the server
//     echoes it back faithfully, so tracking it in state enables drift
//     detection (unlike `Password`, which the server redacts).
//   - `DisplayFields` is Computed-only — the classic API rejects any populated
//     <display_field> (409 "Problem with display_fields"); only the
//     `EnableDisplayFieldsForGroupObject` bool is writable.
type WebhookResourceModel struct {
	ID                                types.String           `tfsdk:"id"`
	Name                              types.String           `tfsdk:"name"`
	Enabled                           types.Bool             `tfsdk:"enabled"`
	URL                               types.String           `tfsdk:"url"`
	AuthenticationType                types.String           `tfsdk:"authentication_type"`
	ConnectionTimeout                 types.Int64            `tfsdk:"connection_timeout"`
	ReadTimeout                       types.Int64            `tfsdk:"read_timeout"`
	ContentType                       types.String           `tfsdk:"content_type"`
	Event                             types.String           `tfsdk:"event"`
	Username                          types.String           `tfsdk:"username"`
	Password                          types.String           `tfsdk:"password"`
	PasswordWoVersion                 types.Int64            `tfsdk:"password_wo_version"`
	Header                            types.String           `tfsdk:"header"`
	HashAlgorithm                     types.String           `tfsdk:"hash_algorithm"`
	SmartGroupID                      types.Int64            `tfsdk:"smart_group_id"`
	EnableDisplayFieldsForGroupObject types.Bool             `tfsdk:"enable_display_fields_for_group_object"`
	DisplayFields                     types.Set              `tfsdk:"display_fields"`
	Timeouts                          resourceTimeouts.Value `tfsdk:"timeouts"`
}

// WebhookDataSourceModel is the flat read-only data source projection. Selects
// by `id` or exact `name` (ExactlyOneOf). `password` is omitted (the server
// redacts it); `header` is surfaced as Sensitive since the server echoes it.
type WebhookDataSourceModel struct {
	ID                                types.String             `tfsdk:"id"`
	Name                              types.String             `tfsdk:"name"`
	Enabled                           types.Bool               `tfsdk:"enabled"`
	URL                               types.String             `tfsdk:"url"`
	AuthenticationType                types.String             `tfsdk:"authentication_type"`
	ConnectionTimeout                 types.Int64              `tfsdk:"connection_timeout"`
	ReadTimeout                       types.Int64              `tfsdk:"read_timeout"`
	ContentType                       types.String             `tfsdk:"content_type"`
	Event                             types.String             `tfsdk:"event"`
	Username                          types.String             `tfsdk:"username"`
	Header                            types.String             `tfsdk:"header"`
	HashAlgorithm                     types.String             `tfsdk:"hash_algorithm"`
	SmartGroupID                      types.Int64              `tfsdk:"smart_group_id"`
	EnableDisplayFieldsForGroupObject types.Bool               `tfsdk:"enable_display_fields_for_group_object"`
	DisplayFields                     types.Set                `tfsdk:"display_fields"`
	Timeouts                          datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// webhookIdentityModel represents the identity object for the resource and list
// results.
type webhookIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// WebhookListResourceModel represents the config model for list queries.
// Classic /webhooks has no RSQL — the filter shape is the shared client-side
// substring block.
type WebhookListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
