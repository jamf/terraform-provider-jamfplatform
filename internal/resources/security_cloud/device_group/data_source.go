// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// DeviceGroupDataSource implements the Terraform data source for a single Jamf
// Security Cloud device group.
type DeviceGroupDataSource struct {
	client *securitycloud.Client
}

var (
	_ datasource.DataSource                     = &DeviceGroupDataSource{}
	_ datasource.DataSourceWithConfigValidators = &DeviceGroupDataSource{}
)

// NewDeviceGroupDataSource returns a new instance of DeviceGroupDataSource.
func NewDeviceGroupDataSource() datasource.DataSource {
	return &DeviceGroupDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DeviceGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_device_group"
}

// Schema returns the data source schema.
func (d *DeviceGroupDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Security Cloud device group by ID or by name. Group names are unique on " +
			"the tenant, but they are matched exactly, so a name that differs only in capitalisation is a " +
			"different group and will not be found." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Device group ID to look up. Exactly one of `id` or `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Device group name to look up. Exactly one of `id` or `name` must be set. The " +
					"built-in \"Default Group\" cannot be looked up here, because it has no ID. Use the " +
					"`jamfplatform_security_cloud_device_groups` data source to see it.",
				Optional: true,
				Computed: true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id or name is supplied.
func (d *DeviceGroupDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *DeviceGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_device_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a device group by ID or name and populates Terraform state.
//
// The name path matches over the v2 list locally rather than through the SDK's
// ResolveDeviceGroupV2ByName, because that resolver cannot express the one result
// this data source has to refuse. The implicit "Default Group" is in the list and
// matches by name, but Jamf Security Cloud gives it no identifier — and the SDK
// resolver turns exactly that case into "matched element has no id field" while
// discarding the matched element, so the refusal below would be unreachable and
// the operator would get an SDK-internal string with no remedy. See
// groupsNamedExactly in helpers.go.
//
// Refusing it matters because a group with no ID cannot be referenced by anything:
// handing back an empty string would produce a config that plans and then fails
// against the API.
//
// The empty-`id` guard exists because ExactlyOneOf counts a set-but-empty string as
// configured, so `id = var.group_id` with an unset variable satisfies the validator
// and then falls through to whichever branch is left. Without the guard the name
// branch would run with a null name and report that no group is named "" — an
// error against an attribute the operator never set.
func (d *DeviceGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DeviceGroupDataSourceModel
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

	var group *securitycloud.Group

	if !data.ID.IsNull() && data.ID.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("id"),
			"Device group ID is empty",
			"`id` is set to an empty string. Jamf Security Cloud has no group with an empty identifier, and an "+
				"empty string still counts as configured, so `name` cannot be used instead. This usually means a "+
				"variable or a reference resolved to \"\" — set `id` to a group ID, or remove it and set `name`.",
		)
		return
	}

	if !data.ID.IsNull() {
		got, err := d.client.GetDeviceGroupV1(readCtx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to find Jamf Security Cloud device group", helpers.APIErrorDetail(err))
			return
		}
		group = got
	} else {
		listed, err := d.client.ListDeviceGroupsV2(readCtx)
		if err != nil {
			resp.Diagnostics.AddError("Unable to find Jamf Security Cloud device group", helpers.APIErrorDetail(err))
			return
		}

		matches := groupsNamedExactly(listed.Groups, data.Name.ValueString())
		switch {
		case len(matches) == 0:
			resp.Diagnostics.AddAttributeError(
				path.Root("name"),
				"Unable to find Jamf Security Cloud device group",
				"No device group on this tenant is named \""+data.Name.ValueString()+"\". Jamf Security Cloud "+
					"compares names exactly, so a group differing only in capitalisation is a different group and "+
					"will not be found here.",
			)
			return
		case len(matches) > 1:
			resp.Diagnostics.AddAttributeError(
				path.Root("name"),
				"Multiple Jamf Security Cloud device groups share that name",
				"Jamf Security Cloud refuses to store two groups under the same name, so this should not be "+
					"reachable. Look the group up by `id` instead.",
			)
			return
		}

		item := matches[0]
		if item.ID == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("name"),
				"Device group has no ID",
				"Jamf Security Cloud gives the built-in group named \""+item.Name+"\" no identifier, so it cannot be "+
					"looked up here or referenced from another resource. Use the "+
					"`jamfplatform_security_cloud_device_groups` data source, which reports it with `built_in` set "+
					"to `true`.",
			)
			return
		}
		group = &securitycloud.Group{ID: item.ID, Name: item.Name}
	}

	assignDeviceGroupDataSourceModel(&data, group)

	tflog.Trace(ctx, "read Jamf Security Cloud device group data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
