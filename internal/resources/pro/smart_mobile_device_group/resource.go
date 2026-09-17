// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package smart_mobile_device_group implements the
// jamfplatform_pro_smart_mobile_device_group resource, its singular and plural
// data sources, and its list resource, backed by
// /pro/v2/mobile-device-groups/smart-groups.
//
// The wire laws this construct rests on are recorded once for the whole family
// in internal/common/progroups (see that package's doc comment), and the helpers
// there are the only place any of them is implemented. Two facts are specific to
// this endpoint and stated here because a reader of this package will not
// otherwise meet them.
//
// # Wire field names
//
// This endpoint prefixes the group's own scalars, which no other construct in
// the provider does, so the Terraform attribute and the wire field disagree for
// three of them:
//
//	| Terraform attribute              | Wire field       |
//	|----------------------------------|------------------|
//	| id                               | groupId          |
//	| name                             | groupName        |
//	| description                      | groupDescription |
//	| site_id                          | siteId           |
//	| criteria                         | criteria         |
//	| member_count (plural DS only)    | count            |
//
// The older group interface the fallback write uses names the same three
// differently again, and carries no description at all:
//
//	| Terraform attribute | Older interface field |
//	|---------------------|-----------------------|
//	| id                  | id                    |
//	| name                | name                  |
//	| description         | (no field)            |
//	| site_id             | site/id, a number     |
//	| criteria            | criteria/criterion    |
//
// So a fallback write maps at both boundaries, and the description it cannot
// carry is set by a separate merge patch on the group endpoint. See crud.go.
//
// platform_id has no field on this endpoint at all. It is the identifier the
// jamfplatform_device_* constructs key on, and it reaches state two ways: the
// create returns it in place of the Jamf Pro id when asked (platform=true), and
// every other path resolves it through progroups.PlatformIDsByJamfProID.
//
// # siteId is mandatory here
//
// Both mobile group endpoints refuse a write that omits siteId, with 403
// INVALID_PRIVILEGE naming the field, while both computer endpoints accept the
// omission and clear the site. progroups.SiteIDForWrite always returns a value,
// so this costs the resource nothing, but a future edit that starts omitting the
// key on a "nothing to change" path would fail in a way that reads as a
// permission problem.
package smart_mobile_device_group

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
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

// minJamfProVersion is empty because there is no floor to source for this
// construct. The version question this family answers is not "which release
// introduced the endpoint" but "which releases return a Jamf-group criterion
// value the provider cannot keep", and that answer lives in
// progroups.RequireSupportedJamfProVersion, which every Configure below calls.
const minJamfProVersion = ""

// Create and update are allowed five minutes. Recomputing membership from the
// criteria is the slow half of a smart-group write, it happens before the
// response, and it grows with the size of the mobile device inventory. The
// 180-second gateway ceiling the SDK records in docs/WIRE-FACTS.md is declared
// on the computer smart-group path rather than this one, so the budget is set
// from the recomputation instead of from a ceiling this endpoint has not been
// observed to carry. Read and delete are one round trip each.
const (
	defaultCreateTimeout = 5 * time.Minute
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 5 * time.Minute
	defaultDeleteTimeout = 60 * time.Second
)

// SmartMobileDeviceGroupResource implements the Terraform resource.
//
// Two clients. Every read and the first attempt at every write go through
// client; classicClient repeats a write the group endpoint refused through Jamf
// Pro's older group interface, and does nothing else. The read deliberately
// stays on the group endpoint on both paths, because it returns these groups and
// their criteria in full.
//
// ldap resolves a directory-service group criterion between the group name a
// practitioner writes and the encoded value Jamf Pro stores. The Pro client
// carries the directory search, so it serves as the resolver directly.
//
// pd carries the cached Jamf Pro version for the family's version refusal, the
// impact-alert cache, and the once-per-run latch the platform_id bridge uses for
// its advisories.
type SmartMobileDeviceGroupResource struct {
	client        *pro.Client
	classicClient *proclassic.Client
	ldap          ldapgroups.Searcher
	pd            *providerdata.Data
}

var (
	_ resource.Resource                     = &SmartMobileDeviceGroupResource{}
	_ resource.ResourceWithImportState      = &SmartMobileDeviceGroupResource{}
	_ resource.ResourceWithIdentity         = &SmartMobileDeviceGroupResource{}
	_ resource.ResourceWithConfigValidators = &SmartMobileDeviceGroupResource{}
	_ resource.ResourceWithModifyPlan       = &SmartMobileDeviceGroupResource{}
)

// NewSmartMobileDeviceGroupResource returns a new instance of the resource.
func NewSmartMobileDeviceGroupResource() resource.Resource {
	return &SmartMobileDeviceGroupResource{}
}

// Metadata sets the resource type name.
func (r *SmartMobileDeviceGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smart_mobile_device_group"
}

// IdentitySchema declares the identifier used for import and list results.
func (r *SmartMobileDeviceGroupResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro identifier for the smart mobile device group.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource.
func (r *SmartMobileDeviceGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(progroups.DeviceTypeMobile, progroups.GroupKindSmart) +
			" Membership is Jamf Pro's to decide: it evaluates the criteria against mobile device inventory and re-evaluates them as that inventory changes, so no member list appears in this configuration." +
			"\n\nJamf Pro offers criterion and operator pairings in its web interface that it will not accept when a group is saved the usual way. On mobile device groups that is `has` on an extension attribute, which Jamf tracks as PI-1032. The provider recognises the refusal and saves the group another way, so this configuration needs no change. One check is weaker on that route: Jamf Pro does not confirm that an operator suits the criterion it is paired with, so a group with no members is worth a second look." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier Jamf Pro assigns to the group. Read-only.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"platform_id": schema.StringAttribute{
				MarkdownDescription: "Identifier for this group across Jamf Platform. A blueprint or a compliance benchmark targets a group by this value, as does anything else outside Jamf Pro that scopes to one. Read-only.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group display name, unique among mobile device groups.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Free-text note stored with the group.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site the group belongs to. `-1` means no site, and is the default.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(progroups.NoSiteID),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"criteria": schema.ListNestedAttribute{
				MarkdownDescription: "Criteria that decide who is in the group. Order is significant: Jamf Pro reads the list from first entry to last, each entry joining the one before it, and `has_opening_parenthesis` / `has_closing_parenthesis` bracket them. Leave the list out to store a group with no criteria.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: criteria.CriterionAttributes(criteria.Operators),
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// Configure wires the two Jamf Pro clients into the resource and applies the
// family's version refusal.
//
// The second client exists for the fallback write alone. It is built here rather
// than lazily so that a configuration that will need it fails at configure time
// with the same diagnostic the first client raises, instead of part-way through
// an apply.
func (r *SmartMobileDeviceGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_smart_mobile_device_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	classicClient, classicDiags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_smart_mobile_device_group")
	resp.Diagnostics.Append(classicDiags...)
	if resp.Diagnostics.HasError() || classicClient == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_smart_mobile_device_group")...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.classicClient = classicClient
	r.ldap = client
	r.pd = pd
}

// ImportState imports by the identifier Jamf Pro assigns to the group.
func (r *SmartMobileDeviceGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ConfigValidators registers the criterion-priority check.
func (r *SmartMobileDeviceGroupResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		criteriaPrioritiesConfigValidator{},
	}
}
