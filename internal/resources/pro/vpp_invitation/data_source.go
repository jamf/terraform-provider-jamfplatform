// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_invitation

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

type VPPInvitationDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &VPPInvitationDataSource{}
	_ datasource.DataSourceWithConfigure        = &VPPInvitationDataSource{}
	_ datasource.DataSourceWithConfigValidators = &VPPInvitationDataSource{}
)

func NewVPPInvitationDataSource() datasource.DataSource {
	return &VPPInvitationDataSource{}
}

func (d *VPPInvitationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_vpp_invitation"
}

func dsComputedString(desc string) dsschema.StringAttribute {
	return dsschema.StringAttribute{MarkdownDescription: desc, Computed: true}
}

func (d *VPPInvitationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Look up a Jamf Pro VPP invitation by ID or name. Exactly one selector must be supplied." + dataSourcePrivileges,
		Attributes: map[string]dsschema.Attribute{
			"id":                          dsschema.StringAttribute{MarkdownDescription: "Invitation ID. Mutually exclusive with `name`.", Optional: true, Computed: true},
			"name":                        dsschema.StringAttribute{MarkdownDescription: "Invitation name (exact match). Mutually exclusive with `id`.", Optional: true, Computed: true},
			"vpp_account_id":              dsComputedString("VPP account ID."),
			"distribution_method":         dsComputedString("Distribution method."),
			"auto_register_managed_users": dsschema.BoolAttribute{MarkdownDescription: "Auto-register Managed Apple ID users.", Computed: true},
			"sender_name":                 dsComputedString("Sender display name (email mode)."),
			"sender_email_address":        dsComputedString("Sender email address (email mode)."),
			"subject":                     dsComputedString("Email subject (email mode)."),
			"message":                     dsComputedString("Email message (email mode)."),
			"require_login":               dsschema.BoolAttribute{MarkdownDescription: "Require login (email mode).", Computed: true},
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
			"invitation_usages": dsschema.ListNestedAttribute{
				MarkdownDescription: "Read-only per-user registration status.",
				Computed:            true,
				NestedObject: dsschema.NestedAttributeObject{
					Attributes: map[string]dsschema.Attribute{
						"id":                     dsComputedString("Usage record ID."),
						"name":                   dsComputedString("User name."),
						"email_address":          dsComputedString("User email address."),
						"status":                 dsComputedString("Registration status."),
						"last_action_date_utc":   dsComputedString("Last action timestamp (UTC)."),
						"last_action_date_epoch": dsComputedString("Last action timestamp (epoch ms)."),
						"vpp_account":            dsComputedString("VPP account name."),
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

func (d *VPPInvitationDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *VPPInvitationDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_vpp_invitation")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

func (d *VPPInvitationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The provider client was not configured.")
		return
	}
	var data VPPInvitationDataSourceModel
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
			resp.Diagnostics.AddError("Unable to find Jamf Pro VPP invitation", helpers.APIErrorDetail(err))
			return
		}
	default:
		resp.Diagnostics.AddError("Missing selector", "Exactly one of id or name must be supplied.")
		return
	}

	got, err := d.client.GetVPPInvitationByID(readCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro VPP invitation", helpers.APIErrorDetail(err))
		return
	}
	assignVPPInvitationDataSourceModel(readCtx, &data, got)

	tflog.Trace(ctx, "read Jamf Pro VPP invitation data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// resolveIDByName lists invitations and returns the ID of the single exact-name
// match. The classic /vppinvitations API has no get-by-name endpoint.
func (d *VPPInvitationDataSource) resolveIDByName(ctx context.Context, name string) (string, error) {
	listed, err := d.client.ListVPPInvitations(ctx)
	if err != nil {
		return "", err
	}
	if listed == nil {
		return "", fmt.Errorf("no VPP invitation named %q found", name)
	}
	var matchID string
	matches := 0
	for _, item := range listed.VppInvitations {
		if helpers.DerefString(item.Name) == name {
			matches++
			matchID = helpers.StringValueFromIntPtr(item.ID).ValueString()
		}
	}
	switch matches {
	case 0:
		return "", fmt.Errorf("no VPP invitation named %q found", name)
	case 1:
		return matchID, nil
	default:
		return "", fmt.Errorf("%d VPP invitations named %q found; look up by id instead", matches, name)
	}
}
