// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// RestrictedSoftwareDataSource implements the Terraform data source for Jamf Pro
// restricted software records. Lookup is by ID or by exact name — exactly one
// must be supplied. The data source surfaces a flat Computed projection; manage
// the record as a resource or import it for full detail.
type RestrictedSoftwareDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &RestrictedSoftwareDataSource{}
	_ datasource.DataSourceWithConfigure        = &RestrictedSoftwareDataSource{}
	_ datasource.DataSourceWithConfigValidators = &RestrictedSoftwareDataSource{}
)

// NewRestrictedSoftwareDataSource returns a new instance of RestrictedSoftwareDataSource.
func NewRestrictedSoftwareDataSource() datasource.DataSource {
	return &RestrictedSoftwareDataSource{}
}

// Metadata sets the data source type name.
func (d *RestrictedSoftwareDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_restricted_software"
}

// Schema returns the data source schema.
func (d *RestrictedSoftwareDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro restricted software record by ID or by exact name. Exactly one of `id` or `name` must be supplied. Surfaces a flat read-only projection; manage the record as a resource for full detail." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Record ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"process_name":                         schema.StringAttribute{MarkdownDescription: "The restricted process name.", Computed: true},
			"restrict_exact_process_name":          schema.BoolAttribute{MarkdownDescription: "Whether only the exact process name is restricted.", Computed: true},
			"send_email_notification_on_violation": schema.BoolAttribute{MarkdownDescription: "Whether an email notification is sent on violation.", Computed: true},
			"kill_process":                         schema.BoolAttribute{MarkdownDescription: "Whether the restricted process is terminated.", Computed: true},
			"delete_application":                   schema.BoolAttribute{MarkdownDescription: "Whether the application running the restricted process is deleted.", Computed: true},
			"display_message":                      schema.StringAttribute{MarkdownDescription: "Message displayed to users when the process is found.", Computed: true},
			"site_id":                              schema.StringAttribute{MarkdownDescription: "Site ID. `-1` means no site.", Computed: true},
			"site_name":                            schema.StringAttribute{MarkdownDescription: "Site display name.", Computed: true},
			"timeouts":                             timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *RestrictedSoftwareDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *RestrictedSoftwareDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_restricted_software")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a record by ID or by name and populates Terraform state.
func (d *RestrictedSoftwareDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data RestrictedSoftwareDataSourceModel
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
		got *proclassic.RestrictedSoftware
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetRestrictedSoftwareByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetRestrictedSoftwareByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing record selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro restricted software", helpers.APIErrorDetail(err))
		return
	}
	assignRestrictedSoftwareFlatDataSource(&data, got)

	tflog.Trace(ctx, "read Jamf Pro restricted software data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// assignRestrictedSoftwareFlatDataSource projects a *proclassic.RestrictedSoftware
// into the flat data source model.
func assignRestrictedSoftwareFlatDataSource(state *RestrictedSoftwareDataSourceModel, rs *proclassic.RestrictedSoftware) {
	if rs == nil {
		return
	}
	if id := extractRestrictedSoftwareID(rs); id != "" {
		state.ID = helpers.StringPointerValueOrNull(&id)
	}
	if rs.General != nil {
		state.Name = helpers.StringPointerValueOrNull(rs.General.Name)
		state.ProcessName = helpers.StringPointerValueOrNull(rs.General.ProcessName)
		state.RestrictExactProcessName = helpers.BoolPointerValueOrNull(rs.General.MatchExactProcessName)
		state.SendEmailNotificationOnViolation = helpers.BoolPointerValueOrNull(rs.General.SendNotification)
		state.KillProcess = helpers.BoolPointerValueOrNull(rs.General.KillProcess)
		state.DeleteApplication = helpers.BoolPointerValueOrNull(rs.General.DeleteExecutable)
		state.DisplayMessage = helpers.StringPointerValueOrNull(rs.General.DisplayMessage)
		if rs.General.Site != nil {
			state.SiteID = helpers.StringValueFromIntPtr(rs.General.Site.ID)
			state.SiteName = helpers.DerivedRefName(rs.General.Site.ID, rs.General.Site.Name)
		}
	}
}
