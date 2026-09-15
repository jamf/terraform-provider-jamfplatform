// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// BenchmarkResource implements the Terraform resource for Jamf Compliance Benchmark.
type BenchmarkResource struct {
	client *compliancebenchmarks.Client
	// impact backs the plan-time impact alert on device group targeting. nil when
	// the provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &BenchmarkResource{}
var _ resource.ResourceWithImportState = &BenchmarkResource{}
var _ resource.ResourceWithIdentity = &BenchmarkResource{}
var _ resource.ResourceWithModifyPlan = &BenchmarkResource{}

const (
	defaultCreateTimeout = 15 * time.Minute
	defaultReadTimeout   = 60 * time.Second
	defaultDeleteTimeout = 15 * time.Minute
)

// uuidRegex matches UUID strings used to validate device group IDs.
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// NewBenchmarkResource returns a new instance of BenchmarkResource.
func NewBenchmarkResource() resource.Resource {
	return &BenchmarkResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *BenchmarkResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cbengine_benchmark"
}

// IdentitySchema defines the unique identifier for benchmark resources.
func (r *BenchmarkResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Benchmark ID used to uniquely reference Jamf Compliance Benchmarks.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the benchmark resource.
func (r *BenchmarkResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = benchmarkResourceSchema(ctx)
}

// benchmarkResourceSchema builds the current resource schema. Factored out of Schema
// so the state upgrader can derive the prior version from it (adding back the one
// attribute that was removed) instead of transcribing 160 lines of nested rule
// attributes, which would then drift from the real schema on the next change.
func benchmarkResourceSchema(ctx context.Context) schema.Schema {
	return schema.Schema{
		Version:             1,
		MarkdownDescription: "Creates a Jamf Compliance Benchmark. Creation is asynchronous: the platform accepts the request and deploys the associated artifacts to the MDM. The provider polls the benchmark's sync state until it reaches `SYNCED` or a terminal failure. Requires **Compliance Benchmarks API** access." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier assigned by the platform.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"title": schema.StringAttribute{
				MarkdownDescription: "Benchmark title (max length 100). Required and replaces the resource when changed.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 100)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional human-readable description of the benchmark (max length 1000). Replaces the resource when changed.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(0, 1000)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source_baseline_id": schema.StringAttribute{
				MarkdownDescription: "mSCP baseline identifier used as the source for rules. Required on creation, but computed for imports. Use the `jamfplatform_cbengine_baselines` data source to look up available baselines.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"sources": schema.ListNestedAttribute{
				MarkdownDescription: "mSCP sources (branch + revision) included in the benchmark. Computed and read-only: the benchmark always spans the full source set of its baseline, so this cannot be configured. Use `selected_os_versions` to choose which operating system versions the benchmark applies to.",
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"branch": schema.StringAttribute{
							MarkdownDescription: "Source branch name.",
							Computed:            true,
						},
						"revision": schema.StringAttribute{
							MarkdownDescription: "Source revision identifier.",
							Computed:            true,
						},
					},
				},
			},
			"selected_os_versions": schema.SetNestedAttribute{
				MarkdownDescription: "Operating system versions the benchmark applies to. Omit it and the benchmark targets every version available for the baseline; name a subset and it targets only those. Immutable (replace on change). Valid values come from `available_os_versions` on the `jamfplatform_cbengine_rules` data source.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
					setplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"os_type": schema.StringAttribute{
							MarkdownDescription: "Operating system type (e.g. `MAC_OS`).",
							Required:            true,
						},
						"os_version": schema.Int64Attribute{
							MarkdownDescription: "Major operating system version (e.g. `26` = macOS Tahoe, `15` = Sequoia, `14` = Sonoma, `13` = Ventura).",
							Required:            true,
						},
					},
				},
			},
			"available_os_versions": schema.ListNestedAttribute{
				MarkdownDescription: "All operating system versions available for the benchmark's baseline. Computed.",
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"os_type": schema.StringAttribute{
							MarkdownDescription: "Operating system type (e.g. `MAC_OS`).",
							Computed:            true,
						},
						"os_version": schema.Int64Attribute{
							MarkdownDescription: "Major operating system version.",
							Computed:            true,
						},
					},
				},
			},
			"rules": schema.ListNestedAttribute{
				MarkdownDescription: "Set of rules to include in the benchmark. Each entry references a rule id and whether it is enabled; additional metadata (title, section, ODV hints) is reported by the platform. Use the `jamfplatform_cbengine_rules` data source to look up available rules.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Rule identifier from the baseline.",
							Required:            true,
						},
						"enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether the rule is enabled in this benchmark.",
							Required:            true,
						},
						// Every field below is read-only server enrichment. Per
						// STYLE_GUIDE §282 each carries a sticky UseStateForUnknown plan
						// modifier so it holds its prior value on a non-refresh plan
						// instead of going Unknown — otherwise the RequiresReplace on the
						// rules list sees a difference and triggers a spurious replace.
						//
						// Deliberate deviation from §290 (which mandates
						// UseNonNullStateForUnknown inside nested collections): §290 guards
						// the append-during-in-place-update case, where a new element's
						// prior state at its index is Null and copying it would trip the
						// consistency check. This resource has NO in-place update — any
						// rules change forces a full replace (fresh create, modifiers do not
						// run) — so appends never occur in place. Plain UseStateForUnknown
						// is therefore safe AND required: many of these fields (the odv_*
						// values) are legitimately Null in state, and NonNull would leave
						// them Unknown, churning the plan into a perpetual replace.
						"section_name": schema.StringAttribute{
							MarkdownDescription: "Section name of the rule from the baseline.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"title": schema.StringAttribute{
							MarkdownDescription: "Rule title resolved from the baseline.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"references": schema.ListAttribute{
							MarkdownDescription: "Reference URLs or identifiers for the rule.",
							ElementType:         types.StringType,
							Computed:            true,
							PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Rule description from the baseline.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"supported_os": schema.ListNestedAttribute{
							MarkdownDescription: "Operating systems supported by the rule.",
							Computed:            true,
							PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"os_type": schema.StringAttribute{
										MarkdownDescription: "Operating system type (e.g. `MAC_OS`, `IOS`).",
										Computed:            true,
									},
									"os_version": schema.Int64Attribute{
										MarkdownDescription: "OS version integer.",
										Computed:            true,
									},
									"management_type": schema.StringAttribute{
										MarkdownDescription: "Management type for the OS.",
										Computed:            true,
									},
								},
							},
						},
						"os_specific_defaults": schema.MapNestedAttribute{
							MarkdownDescription: "OS-specific defaults for the rule.",
							Computed:            true,
							PlanModifiers:       []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"title": schema.StringAttribute{
										MarkdownDescription: "OS-specific rule title.",
										Computed:            true,
									},
									"description": schema.StringAttribute{
										MarkdownDescription: "OS-specific rule description.",
										Computed:            true,
									},
									"odv_value": schema.StringAttribute{
										MarkdownDescription: "Recommended organization-defined value for this OS.",
										Computed:            true,
									},
									"odv_hint": schema.StringAttribute{
										MarkdownDescription: "Hint for the organization-defined value.",
										Computed:            true,
									},
								},
							},
						},
						"odv_value": schema.StringAttribute{
							MarkdownDescription: "Optional organization-defined value to apply for this rule (if applicable).",
							Optional:            true,
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"odv_hint": schema.StringAttribute{
							MarkdownDescription: "Hint for ODV usage.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"odv_placeholder": schema.StringAttribute{
							MarkdownDescription: "Placeholder for ODV input.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"odv_type": schema.StringAttribute{
							MarkdownDescription: "ODV type (`INTEGER`, `STRING`, `ENUM`, `REGEX`) when applicable.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"odv_validation_min": schema.Int64Attribute{
							MarkdownDescription: "Minimum validation for `INTEGER` ODV types.",
							Computed:            true,
							PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
						},
						"odv_validation_max": schema.Int64Attribute{
							MarkdownDescription: "Maximum validation for `INTEGER` ODV types.",
							Computed:            true,
							PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
						},
						"odv_validation_enum_values": schema.ListAttribute{
							MarkdownDescription: "Allowed enum values for `ENUM` ODV types.",
							ElementType:         types.StringType,
							Computed:            true,
							PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
						},
						"odv_validation_regex": schema.StringAttribute{
							MarkdownDescription: "Regex pattern for `REGEX` ODV types.",
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						},
						"depends_on": schema.ListAttribute{
							MarkdownDescription: "Rule IDs this rule depends on.",
							ElementType:         types.StringType,
							Computed:            true,
							PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
						},
						"reportable": schema.BoolAttribute{
							MarkdownDescription: "Whether the rule produces reportable compliance data.",
							Computed:            true,
							PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
						},
						"smart_card": schema.BoolAttribute{
							MarkdownDescription: "Whether the rule is related to smart card configuration.",
							Computed:            true,
							PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
						},
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"target_device_groups": schema.SetAttribute{
				MarkdownDescription: "Device groups this benchmark targets, as a set of group UUIDs. Read them from the `jamfplatform_device_group` or `jamfplatform_device_groups` data sources rather than hand-copying. Immutable (replace on change).",
				ElementType:         types.StringType,
				Required:            true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.RegexMatches(uuidRegex, "Each device group ID must be a valid UUID")),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
			"enforcement_mode": schema.StringAttribute{
				MarkdownDescription: "Enforcement mode for the benchmark; allowed values: MONITOR or MONITOR_AND_ENFORCE. Required and immutable for this resource (replace on change).",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(compliancebenchmarks.BenchmarkRequestV2EnforcementModeMonitor, compliancebenchmarks.BenchmarkRequestV2EnforcementModeMonitorAndEnforce)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// Read-only server-derived fields (STYLE_GUIDE §282): each carries
			// UseStateForUnknown so it holds its prior value on a non-refresh plan
			// rather than going Unknown and provoking a spurious in-place update
			// (which this replace-only resource cannot service). A refresh still
			// re-reads the live value, so drift is caught normally.
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Identifier for the tenant that owns the benchmark.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"deleted": schema.BoolAttribute{
				MarkdownDescription: "Whether the benchmark is marked deleted by the platform.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"update_available": schema.BoolAttribute{
				MarkdownDescription: "Whether an update is available for the benchmark relative to current mSCP sources.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"can_switch_to_enforce": schema.BoolAttribute{
				MarkdownDescription: "Whether the benchmark can be switched to MONITOR_AND_ENFORCE enforcement mode.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"last_updated_at": schema.StringAttribute{
				MarkdownDescription: "Timestamp (RFC3339) of the last update to the benchmark.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Delete: true,
			}),
		},
	}
}

// Configure sets up the API client for the resource from the provider configuration.
func (r *BenchmarkResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_cbengine_benchmark", providerdata.ComplianceBenchmarksScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.impact = pd.ImpactCache()
	r.client = compliancebenchmarks.New(pd.Client)
}

// ImportState handles the import of existing Benchmark resources.
func (r *BenchmarkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
