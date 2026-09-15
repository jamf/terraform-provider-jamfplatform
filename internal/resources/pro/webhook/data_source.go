// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// WebhookDataSource implements the Terraform data source for Jamf Pro webhooks.
// Lookup is by ID or by exact name — exactly one must be supplied. `password`
// is not surfaced (the server redacts it).
type WebhookDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &WebhookDataSource{}
	_ datasource.DataSourceWithConfigure        = &WebhookDataSource{}
	_ datasource.DataSourceWithConfigValidators = &WebhookDataSource{}
)

// NewWebhookDataSource returns a new instance of WebhookDataSource.
func NewWebhookDataSource() datasource.DataSource {
	return &WebhookDataSource{}
}

// Metadata sets the data source type name.
func (d *WebhookDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_webhook"
}

// Schema returns the data source schema.
func (d *WebhookDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro webhook by ID or by exact name. Exactly one of `id` or `name` must be supplied. The plaintext password is never surfaced; `header` is returned (Jamf Pro echoes it) and marked sensitive." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Webhook ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"enabled":                                schema.BoolAttribute{MarkdownDescription: "Whether the webhook is active.", Computed: true},
			"url":                                    schema.StringAttribute{MarkdownDescription: "The URL the payload is posted to.", Computed: true},
			"authentication_type":                    schema.StringAttribute{MarkdownDescription: "Authentication type (NONE, BASIC, HEADER, HASH_SIGNATURE, or MTLS for UI-created webhooks).", Computed: true},
			"connection_timeout":                     schema.Int64Attribute{MarkdownDescription: "Connection timeout in seconds.", Computed: true},
			"read_timeout":                           schema.Int64Attribute{MarkdownDescription: "Read timeout in seconds.", Computed: true},
			"content_type":                           schema.StringAttribute{MarkdownDescription: "Payload content type.", Computed: true},
			"event":                                  schema.StringAttribute{MarkdownDescription: "Triggering event.", Computed: true},
			"username":                               schema.StringAttribute{MarkdownDescription: "BASIC authentication username.", Computed: true},
			"header":                                 schema.StringAttribute{MarkdownDescription: "HEADER authentication JSON metadata.", Computed: true, Sensitive: true},
			"hash_algorithm":                         schema.StringAttribute{MarkdownDescription: "HASH_SIGNATURE algorithm.", Computed: true},
			"smart_group_id":                         schema.Int64Attribute{MarkdownDescription: "Associated smart group ID for SmartGroup* events (null otherwise).", Computed: true},
			"enable_display_fields_for_group_object": schema.BoolAttribute{MarkdownDescription: "Whether display fields are included for the group object.", Computed: true},
			"display_fields":                         schema.SetAttribute{MarkdownDescription: "Display field names included in the group-object payload.", ElementType: types.StringType, Computed: true},
			"timeouts":                               timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *WebhookDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *WebhookDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_webhook")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a record by ID or by name and populates Terraform state.
func (d *WebhookDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data WebhookDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	var (
		got *proclassic.Webhook
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetWebhookByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetWebhookByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing record selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro webhook", helpers.APIErrorDetail(err))
		return
	}
	assignWebhookFlatDataSource(ctx, &data, got)

	tflog.Trace(ctx, "read Jamf Pro webhook data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// assignWebhookFlatDataSource projects a *proclassic.Webhook into the flat data
// source model.
func assignWebhookFlatDataSource(ctx context.Context, state *WebhookDataSourceModel, w *proclassic.Webhook) {
	if w == nil {
		return
	}
	if id := extractWebhookID(w); id != "" {
		state.ID = types.StringValue(id)
	}
	state.Name = helpers.StringPointerValueOrNull(w.Name)
	state.Enabled = helpers.BoolPointerValueOrNull(w.Enabled)
	state.URL = helpers.StringPointerValueOrNull(w.URL)
	state.AuthenticationType = helpers.StringPointerValueOrNull(w.AuthenticationType)
	state.ConnectionTimeout = helpers.Int64FromIntPtr(w.ConnectionTimeout)
	state.ReadTimeout = helpers.Int64FromIntPtr(w.ReadTimeout)
	state.ContentType = helpers.StringPointerValueOrNull(w.ContentType)
	state.Event = helpers.StringPointerValueOrNull(w.Event)
	state.Username = emptyStringToNull(w.Username)
	state.Header = emptyStringToNull(w.Header)
	state.HashAlgorithm = helpers.StringPointerValueOrNull(w.HashAlgorithm)
	state.SmartGroupID = smartGroupIDToState(w.SmartGroupID)
	state.EnableDisplayFieldsForGroupObject = helpers.BoolPointerValueOrNull(w.EnableDisplayFieldsForGroupObject)
	state.DisplayFields = flattenStringSet(ctx, displayFieldNames(w.DisplayFields))
}
