// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_assignment

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

type VPPAssignmentDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &VPPAssignmentDataSource{}
	_ datasource.DataSourceWithConfigure        = &VPPAssignmentDataSource{}
	_ datasource.DataSourceWithConfigValidators = &VPPAssignmentDataSource{}
)

func NewVPPAssignmentDataSource() datasource.DataSource {
	return &VPPAssignmentDataSource{}
}

func (d *VPPAssignmentDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_vpp_assignment"
}

func dsContentList(desc string) dsschema.ListNestedAttribute {
	return dsschema.ListNestedAttribute{
		MarkdownDescription: desc,
		Computed:            true,
		NestedObject: dsschema.NestedAttributeObject{
			Attributes: map[string]dsschema.Attribute{
				"adam_id": dsschema.Int64Attribute{MarkdownDescription: "Apple catalog adam ID.", Computed: true},
				"name":    dsschema.StringAttribute{MarkdownDescription: "Title name, resolved by Jamf Pro.", Computed: true},
			},
		},
	}
}

func (d *VPPAssignmentDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Look up a Jamf Pro VPP assignment by ID or name. Exactly one selector must be supplied." + dataSourcePrivileges,
		Attributes: map[string]dsschema.Attribute{
			"id":                     dsschema.StringAttribute{MarkdownDescription: "Assignment ID. Mutually exclusive with `name`.", Optional: true, Computed: true},
			"name":                   dsschema.StringAttribute{MarkdownDescription: "Assignment name (exact match). Mutually exclusive with `id`.", Optional: true, Computed: true},
			"vpp_admin_account_id":   dsschema.StringAttribute{MarkdownDescription: "VPP account (Location) ID.", Computed: true},
			"vpp_admin_account_name": dsschema.StringAttribute{MarkdownDescription: "VPP account (Location) display name.", Computed: true},
			"ios_apps":               dsContentList("Assigned iOS apps."),
			"mac_apps":               dsContentList("Assigned Mac apps."),
			"ebooks":                 dsContentList("Assigned books."),
			"scope": dsschema.SingleNestedAttribute{
				MarkdownDescription: "User-based scope.",
				Computed:            true,
				Attributes: map[string]dsschema.Attribute{
					"targets": dsschema.SingleNestedAttribute{
						MarkdownDescription: "Scope targets.",
						Computed:            true,
						Attributes: map[string]dsschema.Attribute{
							"all_jss_users":      dsschema.BoolAttribute{MarkdownDescription: "Target all Jamf Pro users.", Computed: true},
							"jss_user_ids":       dsComputedStringSet("Jamf Pro user IDs."),
							"jss_user_group_ids": dsComputedStringSet("Jamf Pro user group IDs."),
						},
					},
					"limitations": dsschema.SingleNestedAttribute{
						MarkdownDescription: "Scope limitations.",
						Computed:            true,
						Attributes: map[string]dsschema.Attribute{
							"directory_service_user_group_names": dsComputedStringSet("Directory-service user group names."),
						},
					},
					"exclusions": dsschema.SingleNestedAttribute{
						MarkdownDescription: "Scope exclusions.",
						Computed:            true,
						Attributes: map[string]dsschema.Attribute{
							"jss_user_ids":                       dsComputedStringSet("Jamf Pro user IDs."),
							"jss_user_group_ids":                 dsComputedStringSet("Jamf Pro user group IDs."),
							"directory_service_user_group_names": dsComputedStringSet("Directory-service user group names."),
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

func dsComputedStringSet(desc string) dsschema.SetAttribute {
	return dsschema.SetAttribute{MarkdownDescription: desc, Computed: true, ElementType: types.StringType}
}

func (d *VPPAssignmentDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *VPPAssignmentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_vpp_assignment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

func (d *VPPAssignmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The provider client was not configured.")
		return
	}
	var data VPPAssignmentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, d2 := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(d2...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	var id string
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		id = data.ID.ValueString()
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		var err error
		id, err = d.resolveIDByName(readCtx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to find Jamf Pro VPP assignment", helpers.APIErrorDetail(err))
			return
		}
	default:
		resp.Diagnostics.AddError("Missing selector", "Exactly one of id or name must be supplied.")
		return
	}

	got, err := d.client.GetVPPAssignmentByID(readCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro VPP assignment", helpers.APIErrorDetail(err))
		return
	}
	assignVPPAssignmentDataSourceModel(readCtx, &data, got)

	tflog.Trace(ctx, "read Jamf Pro VPP assignment data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// resolveIDByName lists assignments and returns the ID of the single exact-name
// match. The classic /vppassignments API has no get-by-name endpoint.
func (d *VPPAssignmentDataSource) resolveIDByName(ctx context.Context, name string) (string, error) {
	listed, err := d.client.ListVPPAssignments(ctx)
	if err != nil {
		return "", err
	}
	if listed == nil {
		return "", fmt.Errorf("no VPP assignment named %q found", name)
	}
	var matchID string
	matches := 0
	for _, item := range listed.VppAssignments {
		if helpers.DerefString(item.Name) == name {
			matches++
			matchID = helpers.StringValueFromIntPtr(item.ID).ValueString()
		}
	}
	switch matches {
	case 0:
		return "", fmt.Errorf("no VPP assignment named %q found", name)
	case 1:
		return matchID, nil
	default:
		return "", fmt.Errorf("%d VPP assignments named %q found; look up by id instead", matches, name)
	}
}
