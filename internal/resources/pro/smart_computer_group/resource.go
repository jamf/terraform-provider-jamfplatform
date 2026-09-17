// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package smart_computer_group implements the
// jamfplatform_pro_smart_computer_group resource, its singular and plural data
// sources and its list resource: a criteria-driven computer group that can
// belong to a Jamf Pro site.
//
// The wire laws this family shares live in internal/common/progroups, in that
// package's doc comment, and are not restated here. Four facts belong to this
// construct alone.
//
// GET /pro/v3/computer-groups/smart-groups/{id} carries NO identifier of any
// kind: name, description, siteId and criteria come back and nothing else. That
// is why Create calls progroups.JamfProIDForPlatformID — this is the one
// construct in the family that needs the bridge, because a create asking for the
// platform UUID has no other route to the numeric id it must store — and why
// Read carries the prior id through unchanged instead of taking it from the
// response.
//
// Tyk enforces a 180-second hard timeout on a write to this surface in all three
// regions (recorded in the SDK's docs/WIRE-FACTS.md), so the create and update
// budgets are five minutes rather than the house minute. A 60-second provider
// timeout would cancel a write the gateway is still willing to wait on.
//
// Criteria priorities must run 0..n-1 in list order;
// PUT with a lone criterion at priority 5 answers 400
// INVALID_REQUEST_PARAMETER_VALUE on field priority. criteria.BuildComputerSmartGroupCriteria
// numbers from the element index and criteriaPriorityValidator refuses an
// authored value that disagrees, rather than overriding it into a post-apply
// inconsistency.
//
// Jamf Pro refuses criterion and operator combinations its own admin UI offers
// and its older group interface stores, so a refused write is repeated through
// that older interface and the resource needs a proclassic client as well as a
// pro one. internal/common/progroups/fallback.go is the authority on which
// refusals qualify and why they are safe to retry; the head of crud.go carries
// how this package does it. The resource description says as much in product
// terms, including the one check that is weaker on that path.
//
// Terraform attribute names match the wire field names throughout, apart from
// the two identifiers and the site:
//
//	| Terraform   | Wire                                        |
//	|-------------|---------------------------------------------|
//	| id          | the Jamf Pro numeric group id, not in any    |
//	|             | group response — see above                   |
//	| platform_id | the group's Jamf Platform UUID, returned by  |
//	|             | the create when platform=true                |
//	| site_id     | siteId                                      |
package smart_computer_group

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is empty because there is no floor to source. Jamf never
// published one for this surface, and the version decision this construct does
// make is a refusal rather than a minimum: progroups.RequireSupportedJamfProVersion
// rejects the window in which a criterion matching on membership of another Jamf
// group reads back as a numeric id. Configure applies both.
const minJamfProVersion = ""

// The kinds every progroups helper in this package dispatches on. Declared once
// so a copy of this package for another device type or group kind is two edits.
const (
	deviceType = progroups.DeviceTypeComputer
	groupKind  = progroups.GroupKindSmart
)

// dsGroupObjectType is the inventory object class this construct targets, used
// to dispatch the per-class directory-service group criterion allowlist. A
// computer group accepts all five names.
const dsGroupObjectType = criteria.ObjectTypeComputer

// groupLabel names the object the way the Jamf Pro admin UI does, for every
// diagnostic and impact alert this package raises.
var groupLabel = progroups.Label(deviceType, groupKind)

// terraformName is the construct's Terraform type name, passed to both Configure
// gates.
const terraformName = "jamfplatform_pro_smart_computer_group"

// Create and update get five minutes because Tyk allows a write on this surface
// 180 seconds before it cuts the connection; the house minute would cancel a
// write the gateway is still waiting on. Read and delete keep the house minute.
const (
	defaultCreateTimeout = 5 * time.Minute
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 5 * time.Minute
	defaultDeleteTimeout = 60 * time.Second
)

// SmartComputerGroupResource implements the Terraform resource for a Jamf Pro
// smart computer group.
type SmartComputerGroupResource struct {
	client *pro.Client
	// classicClient carries the fallback write path only. Jamf Pro's group
	// endpoints refuse criterion and operator combinations its own admin UI
	// offers and its older group interface stores, so a refused write is
	// repeated there — see the head of crud.go.
	classicClient *proclassic.Client
	// pd carries the cached Jamf Pro version, the impact-alert cache and the
	// once-per-run warning latches the platform identifier bridge uses.
	pd *providerdata.Data
	// ldap resolves a directory-service group criterion between a group name and
	// the base64 {uuid,serverId} value Jamf Pro stores.
	ldap ldapgroups.Searcher
}

var (
	_ resource.Resource                     = &SmartComputerGroupResource{}
	_ resource.ResourceWithImportState      = &SmartComputerGroupResource{}
	_ resource.ResourceWithIdentity         = &SmartComputerGroupResource{}
	_ resource.ResourceWithConfigValidators = &SmartComputerGroupResource{}
	_ resource.ResourceWithModifyPlan       = &SmartComputerGroupResource{}
)

// NewSmartComputerGroupResource returns a new instance of SmartComputerGroupResource.
func NewSmartComputerGroupResource() resource.Resource {
	return &SmartComputerGroupResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *SmartComputerGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smart_computer_group"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *SmartComputerGroupResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro identifier of the smart computer group.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the smart computer group resource.
func (r *SmartComputerGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(deviceType, groupKind) +
			"\n\nManages a smart computer group in Jamf Pro. Jamf Pro decides the membership by evaluating `criteria` against computer inventory on its own schedule, so the members are never listed here and a plan cannot report who will be in the group after an apply.\n\n" +
			"Criteria order carries meaning. Jamf Pro reads the list from the top and joins each entry to the one before it with that entry's `and_or` value, so the first entry's `and_or` is never used and moving an entry changes the result. The parenthesis flags are honoured, so a pair of them groups the entries between them. Priorities have to run from 0 upwards with no gaps, which the list position supplies whenever `priority` is omitted.\n\n" +
			"Jamf Pro offers some criterion and operator pairings in its web interface that it will not accept when a group is saved the usual way. Two are confirmed: a patch reporting criterion, and `has` on an extension attribute. The provider recognises the refusal and saves such a group another way, so this configuration needs no change. One check is weaker on that route. Jamf Pro does not confirm that an operator suits the criterion it is paired with, so a group that ends up with no members is worth a second look. Jamf tracks these as PI-1183 and PI-1032." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro identifier for this group, assigned when the group is created. Use it wherever another Jamf Pro object takes a computer group identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"platform_id": schema.StringAttribute{
				MarkdownDescription: "Identifier for this group across Jamf Platform. Use it to target the group from a blueprint or a compliance benchmark, and anywhere else a `jamfplatform_device_*` construct asks for one. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group name as it appears in Jamf Pro. Jamf Pro requires it to be unique and refuses a name already in use.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Free-text note stored beside the group. Dropping the attribute from a configuration that once set it keeps the stored text; set it to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site the group belongs to. `" + progroups.NoSiteID + "` means no site, which is the default. A smart group in a site only ever contains computers assigned to that same site.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(progroups.NoSiteID),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"criteria": schema.ListNestedAttribute{
				MarkdownDescription: "Criteria Jamf Pro evaluates to decide who is in the group, in evaluation order. A criterion naming a directory service group takes the group name and the provider resolves it, or the stored reference value if you already have one. Omit the attribute for a group with no criteria; an empty list is refused, because Jamf Pro reports such a group with no criteria at all and a configuration asking for an empty one would never settle.",
				Optional:            true,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: criteria.CriterionAttributes(criteria.Operators),
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create:            true,
				Read:              true,
				Update:            true,
				Delete:            true,
				CreateDescription: "How long to allow for creating the group. Defaults to 5m. Jamf Pro evaluates the criteria as part of the write, so a large estate or a broad criterion can need longer.",
				UpdateDescription: "How long to allow for updating the group. Defaults to 5m. Jamf Pro re-evaluates the criteria as part of the write, so a large estate or a broad criterion can need longer.",
			}),
		},
	}
}

// Configure wires both clients into the resource through the shared
// providerdata helpers, then applies the family's version refusal.
//
// The second client is not optional and is not gated on whether a configuration
// looks like it needs the fallback: the trigger is a refusal from Jamf Pro at
// apply time, and there is nothing at configure time that predicts it.
func (r *SmartComputerGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, terraformName)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	classicClient, classicDiags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, terraformName)
	resp.Diagnostics.Append(classicDiags...)
	if resp.Diagnostics.HasError() || classicClient == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, terraformName)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.classicClient = classicClient
	r.pd = pd
	r.ldap = client
}

// ImportState handles import by the Jamf Pro identifier.
func (r *SmartComputerGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ConfigValidators registers the plan-time criterion priority check.
func (r *SmartComputerGroupResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		criteriaPriorityValidator{},
	}
}
