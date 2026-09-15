// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_connect

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// JamfConnectDataSource implements the singular Jamf Connect data source.
// Lookup is by exactly one of config_profile_uuid, profile_id, or
// profile_name.
type JamfConnectDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &JamfConnectDataSource{}
	_ datasource.DataSourceWithConfigure        = &JamfConnectDataSource{}
	_ datasource.DataSourceWithConfigValidators = &JamfConnectDataSource{}
)

// NewJamfConnectDataSource returns a new instance of JamfConnectDataSource.
func NewJamfConnectDataSource() datasource.DataSource {
	return &JamfConnectDataSource{}
}

// Metadata sets the data source type name.
func (d *JamfConnectDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_jamf_connect"
}

// Schema returns the data source schema.
func (d *JamfConnectDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up the Jamf Connect deployment and update settings for a configuration profile that is linked to Jamf Connect (Settings → Jamf apps → Jamf Connect). Supply exactly one of `config_profile_uuid`, `profile_id`, or `profile_name`." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"config_profile_uuid": schema.StringAttribute{
				MarkdownDescription: "Jamf Connect profile UUID. Supply this, `profile_id`, or `profile_name`.",
				Optional:            true,
				Computed:            true,
			},
			"profile_id": schema.Int64Attribute{
				MarkdownDescription: "Jamf Pro ID of the configuration profile. Supply this, `config_profile_uuid`, or `profile_name`.",
				Optional:            true,
				Computed:            true,
			},
			"profile_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the configuration profile (exact match). Supply this, `config_profile_uuid`, or `profile_id`.",
				Optional:            true,
				Computed:            true,
			},
			"auto_deployment_type": schema.StringAttribute{
				MarkdownDescription: "How Jamf Connect is deployed and updated on the profile (`NONE`, `INITIAL_INSTALLATION_ONLY`, `PATCH_UPDATES`, or `MINOR_AND_PATCH_UPDATES`).",
				Computed:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Jamf Connect version configured for deployment. Empty when `auto_deployment_type` is `NONE`.",
				Computed:            true,
			},
			"scope_description": schema.StringAttribute{
				MarkdownDescription: "Human-readable summary of the configuration profile's scope.",
				Computed:            true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "ID of the site the configuration profile belongs to. `-1` means none.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one lookup key is supplied.
func (d *JamfConnectDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("config_profile_uuid"),
			path.MatchRoot("profile_id"),
			path.MatchRoot("profile_name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *JamfConnectDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_jamf_connect")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read finds the linked profile by the supplied key and populates state.
func (d *JamfConnectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data JamfConnectDataSourceModel
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

	profiles, err := d.client.ListJamfConnectConfigProfilesV1(readCtx, nil, "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Connect configuration profiles", helpers.APIErrorDetail(err))
		return
	}

	match, selector := matchJamfConnectProfile(data, profiles)
	if match == nil {
		resp.Diagnostics.AddError(
			"Jamf Connect configuration profile not found",
			fmt.Sprintf("No Jamf Connect-linked configuration profile matched %s.", selector),
		)
		return
	}

	assignJamfConnectDataSourceModel(&data, match)

	tflog.Trace(ctx, "read Jamf Connect data source", map[string]any{"selector": selector})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// matchJamfConnectProfile returns the profile matching the one configured
// lookup key, plus a human-readable description of the selector for errors.
func matchJamfConnectProfile(data JamfConnectDataSourceModel, profiles []pro.LinkedConnectProfile) (*pro.LinkedConnectProfile, string) {
	switch {
	case !data.ConfigProfileUUID.IsNull() && data.ConfigProfileUUID.ValueString() != "":
		want := data.ConfigProfileUUID.ValueString()
		for i := range profiles {
			if helpers.DerefString(profiles[i].UUID) == want {
				return &profiles[i], fmt.Sprintf("config_profile_uuid %q", want)
			}
		}
		return nil, fmt.Sprintf("config_profile_uuid %q", want)
	case !data.ProfileID.IsNull():
		want := data.ProfileID.ValueInt64()
		for i := range profiles {
			if profiles[i].ProfileID != nil && int64(*profiles[i].ProfileID) == want {
				return &profiles[i], fmt.Sprintf("profile_id %d", want)
			}
		}
		return nil, fmt.Sprintf("profile_id %d", want)
	default:
		want := data.ProfileName.ValueString()
		for i := range profiles {
			if helpers.DerefString(profiles[i].ProfileName) == want {
				return &profiles[i], fmt.Sprintf("profile_name %q", want)
			}
		}
		return nil, fmt.Sprintf("profile_name %q", want)
	}
}
