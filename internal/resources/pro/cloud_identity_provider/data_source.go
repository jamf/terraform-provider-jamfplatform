// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//
//	pro.ListCloudIdpV1
//
// Status: current. Last reviewed 2026-05-30.
package cloud_identity_provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// CloudIdentityProviderDataSource implements the Terraform data source for the Jamf Pro
// Cloud Identity Provider registry. The singular data source supports lookup
// by ID OR by display_name — exactly one of the two must be supplied.
//
// Both lookup branches call ListCloudIdpV1 and match client-side so that all
// five registry fields (including enabled and provider_description) are
// available. GetCloudIdpV1 returns the CloudIDPCommon shape, which omits those
// two fields.
type CloudIdentityProviderDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &CloudIdentityProviderDataSource{}
	_ datasource.DataSourceWithConfigure        = &CloudIdentityProviderDataSource{}
	_ datasource.DataSourceWithConfigValidators = &CloudIdentityProviderDataSource{}
)

// NewCloudIdentityProviderDataSource returns a new instance of CloudIdentityProviderDataSource.
func NewCloudIdentityProviderDataSource() datasource.DataSource {
	return &CloudIdentityProviderDataSource{}
}

// Metadata sets the data source type name.
func (d *CloudIdentityProviderDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_cloud_identity_provider"
}

// Schema returns the data source schema.
func (d *CloudIdentityProviderDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro Cloud Identity Provider registry entry by `id` or by exact `display_name`. " +
			"Exactly one of the two must be supplied. Covers both Google (Secure LDAP) and Microsoft Entra ID providers. " +
			"To retrieve the full provider-specific configuration (LDAP server settings, mappings, etc.), use the managed resource instead." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Cloud Identity Provider ID assigned by Jamf Pro. Mutually exclusive with `display_name`.",
				Optional:            true,
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Cloud Identity Provider display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Cloud identity provider type (`GOOGLE` or `ENTRA_ID`).",
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the Cloud Identity Provider is enabled.",
				Computed:            true,
			},
			"provider_description": schema.StringAttribute{
				MarkdownDescription: "Human-readable description of the provider type.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / display_name is supplied.
func (d *CloudIdentityProviderDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("display_name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *CloudIdentityProviderDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_cloud_identity_provider")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches all registry entries from ListCloudIdpV1 and matches client-side
// by id or display_name, then populates state.
func (d *CloudIdentityProviderDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data CloudIdentityProviderDataSourceModel
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

	all, err := d.client.ListCloudIdpV1(readCtx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro Cloud Identity Providers", helpers.APIErrorDetail(err))
		return
	}

	var matched *pro.CloudIDPCommonResponse

	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		lookupID := data.ID.ValueString()
		for i := range all {
			if all[i].ID == lookupID {
				matched = &all[i]
				break
			}
		}
		if matched == nil {
			resp.Diagnostics.AddError(
				"Cloud Identity Provider not found",
				fmt.Sprintf("No Cloud Identity Provider with id %q exists on this tenant.", lookupID),
			)
			return
		}

	case !data.DisplayName.IsNull() && data.DisplayName.ValueString() != "":
		lookupName := data.DisplayName.ValueString()
		var hits []pro.CloudIDPCommonResponse
		for _, item := range all {
			if item.DisplayName == lookupName {
				hits = append(hits, item)
			}
		}
		switch len(hits) {
		case 0:
			resp.Diagnostics.AddError(
				"Cloud Identity Provider not found",
				fmt.Sprintf("No Cloud Identity Provider with display_name %q exists on this tenant.", lookupName),
			)
			return
		case 1:
			matched = &hits[0]
		default:
			resp.Diagnostics.AddError(
				"Ambiguous Cloud Identity Provider display_name",
				fmt.Sprintf("%d Cloud Identity Providers share display_name %q. Use id to identify a specific provider.", len(hits), lookupName),
			)
			return
		}

	default:
		resp.Diagnostics.AddError("Missing Cloud Identity Provider selector", "Exactly one of id or display_name must be supplied.")
		return
	}

	data.ID = types.StringValue(matched.ID)
	data.DisplayName = types.StringValue(matched.DisplayName)
	data.ProviderName = types.StringValue(providerNameFromWire(matched.ProviderName))
	data.Enabled = types.BoolValue(matched.Enabled)
	data.ProviderDescription = types.StringValue(matched.ProviderDescription)

	tflog.Trace(ctx, "read Jamf Pro Cloud Identity Provider data source", map[string]any{
		"id":           data.ID.ValueString(),
		"display_name": data.DisplayName.ValueString(),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
