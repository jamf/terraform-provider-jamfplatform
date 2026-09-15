// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package advanced_user_search implements the
// jamfplatform_pro_advanced_user_search resource, data source, and list resource
// backed by the Jamf ProClassic advancedusersearches API.
package advanced_user_search

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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /advancedusersearches predates the provider's overall
// floor.
const minJamfProVersion = ""

// AdvancedUserSearchResource implements the Terraform resource for Jamf Pro
// advanced user searches.
type AdvancedUserSearchResource struct {
	client *proclassic.Client
	// ldap resolves directory-service group criterion names to/from the base64
	// {uuid,serverId} wire value. Built from the shared Pro client because the
	// classic API has no LDAP-group search of its own.
	ldap ldapgroups.Searcher
}

var _ resource.Resource = &AdvancedUserSearchResource{}
var _ resource.ResourceWithImportState = &AdvancedUserSearchResource{}
var _ resource.ResourceWithIdentity = &AdvancedUserSearchResource{}
var _ resource.ResourceWithModifyPlan = &AdvancedUserSearchResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewAdvancedUserSearchResource returns a new instance of the resource.
func NewAdvancedUserSearchResource() resource.Resource {
	return &AdvancedUserSearchResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *AdvancedUserSearchResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_advanced_user_search"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *AdvancedUserSearchResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro advanced user search ID used to uniquely reference the search.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the advanced user search resource.
func (r *AdvancedUserSearchResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro advanced user search, a saved criteria-driven user query with a configurable set of display columns. The matched-user result set is report data Jamf Pro computes, and is intentionally not modelled. Unlike advanced computer searches, user searches have no `view_as` or sort columns. Mirrors the Users → Search Users → Advanced User Search UI." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Advanced user search ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Advanced user search display name. Must be unique within the tenant.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Optional Jamf Pro site ID to scope the search. Omit to leave unscoped (server sets the `NONE` site, id `-1`).",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(noSiteID),
			},
			"site_name": schema.StringAttribute{
				MarkdownDescription: "Site name reported by Jamf Pro for the assigned `site_id`. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"criteria": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered list of criteria Jamf Pro evaluates to populate the search. Order matters: Jamf Pro reads left to right, applying the supplied `and_or` joins and parentheses. Omit the attribute, or supply an empty list, for a search with no criteria.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: criteria.CriterionAttributes(ValidOperators),
				},
			},
			"display_fields": schema.SetAttribute{
				MarkdownDescription: "Set of column names shown in the search results, for example `Full Name`, `Email Address`, `Username`. Order is not significant, because Jamf Pro returns the columns in its own canonical order. Omit for no display columns.",
				Optional:            true,
				ElementType:         types.StringType,
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

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *AdvancedUserSearchResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_advanced_user_search")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	if pd, ok := req.ProviderData.(*providerdata.Data); ok {
		r.ldap = pro.New(pd.Client)
	}
}

// ImportState imports an advanced user search by ID.
func (r *AdvancedUserSearchResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
