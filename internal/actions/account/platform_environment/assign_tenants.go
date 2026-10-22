// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package platformenvironmentaction implements the Jamf Account platform
// environment action jamfplatform_account_platform_environment_assign_tenants,
// which adds tenants to a platform environment.
//
// Assignment is an action rather than part of the
// jamfplatform_account_platform_environment resource's lifecycle for three
// reasons the wire settled (SDK documentation and the 2026-10-22 probe of
// /organization/v1):
//
//   - It cannot be undone. No call takes a tenant out of a platform environment,
//     and a tenant that is already in one is refused. A resource attribute would
//     imply that removing a tenant from the configuration unassigns it.
//   - It cannot be read back from the platform environment. The environment
//     object carries no tenant list, because membership is recorded on the
//     tenant. There is nothing for a resource to refresh or to drift against.
//   - It makes the platform environment undeletable. A platform environment
//     that holds a tenant cannot be deleted, so assignment changes what the
//     resource's own destroy can do.
//
// The server's refusals are unattributed: an unknown tenant answers
// 400 UNKNOWN_TENANT with no field, and an unassigned-only or region mismatch is
// not reported per tenant. So the action reads the platform environment and the
// tenants first, refuses with every offending tenant named before anything is
// sent, and reads the tenants back afterwards instead of trusting the 204.
//
// SDK endpoints used:
//
//	account.AssignTenants
//	account.ListEnvironments (the target must exist; its region constrains the tenants)
//	account.ListTenants      (pre-check, and read-back after the write)
//
// There is no acceptance test, by maintainer decision: a real assignment cannot
// be reversed, so the action is covered by unit tests against a stub server.
//
// Status: current. Last reviewed 2026-10-22.
package platformenvironmentaction

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accounttenant"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

var (
	_ action.Action              = (*AssignTenantsAction)(nil)
	_ action.ActionWithConfigure = (*AssignTenantsAction)(nil)
)

// AssignTenantsAction adds tenants to a Jamf Account platform environment.
type AssignTenantsAction struct {
	client *account.Client
}

// AssignTenantsActionModel is the action's configuration.
type AssignTenantsActionModel struct {
	EnvironmentID types.String         `tfsdk:"environment_id"`
	TenantIDs     types.Set            `tfsdk:"tenant_ids"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

// NewAssignTenantsAction returns a new instance of AssignTenantsAction.
func NewAssignTenantsAction() action.Action {
	return &AssignTenantsAction{}
}

// Metadata sets the action type name for the Terraform provider.
func (a *AssignTenantsAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_platform_environment_assign_tenants"
}

// Schema returns the action schema.
func (a *AssignTenantsAction) Schema(ctx context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Adds tenants to a platform environment, as the **Services available to add to this " +
			"platform environment** list on the platform environment's page in Jamf Account does.\n\n" +
			"Assigning a tenant cannot be undone. Neither Jamf Account nor this provider takes a tenant back out of " +
			"a platform environment, and a platform environment that holds a tenant cannot be deleted. Check " +
			"`tenant_ids` before you invoke it.\n\n" +
			"Only an unassigned tenant can be added, and its region has to match the platform environment's " +
			"region unless the platform environment is global. The action checks both before it sends anything, " +
			"checks that every tenant exists, and names each tenant that fails. A tenant already in the platform " +
			"environment is skipped, so invoking the action again changes nothing. Afterwards it reads the " +
			"tenants back and fails unless each one reports the platform environment.\n\n" +
			"Find unassigned tenants with the `jamfplatform_account_tenants` data source. Invoke the action by " +
			"name: `terraform apply -invoke='action.jamfplatform_account_platform_environment_assign_tenants.prod'`." +
			assignTenantsPrivileges,
		Attributes: map[string]actionschema.Attribute{
			"environment_id": actionschema.StringAttribute{
				MarkdownDescription: "Identifier of the platform environment to add the tenants to, as exported by " +
					"the `id` attribute of `jamfplatform_account_platform_environment`.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"tenant_ids": actionschema.SetAttribute{
				MarkdownDescription: "Identifiers of the tenants to add, as listed by the `id` attribute of the " +
					"`jamfplatform_account_tenants` data source. At least one.",
				ElementType: types.StringType,
				Required:    true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
		},
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

// Configure wires the Jamf Account client into the action.
//
// It goes through ConfigureAccount rather than ConfigurePro: Jamf Account is
// organization-level, has no customer-tenant version to gate on, and an
// organization need hold no Jamf Pro tenant at all.
func (a *AssignTenantsAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_platform_environment_assign_tenants")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.client = client
}

// Invoke checks the assignment, sends it, and confirms it took.
//
// The 204 the write answers is not the outcome, so the tenants are read back and
// each one has to report the target platform environment.
func (a *AssignTenantsAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if a.client == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The Jamf Account client was not configured. Re-run terraform init/apply so the provider can "+
				"configure successfully.",
		)
		return
	}

	var data AssignTenantsActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var requested []string
	resp.Diagnostics.Append(data.TenantIDs.ElementsAs(ctx, &requested, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sort.Strings(requested)

	environmentID := data.EnvironmentID.ValueString()

	environments, err := a.client.ListEnvironments(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read this organization's platform environments", helpers.APIErrorDetail(err))
		return
	}
	environment := findEnvironment(environments, environmentID)
	if environment == nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("environment_id"),
			"Platform environment not found",
			fmt.Sprintf("Your organization has no platform environment with the identifier \"%s\". Use the "+
				"`jamfplatform_account_platform_environments` data source to list the ones it holds.", environmentID),
		)
		return
	}

	tenants, err := a.client.ListTenants(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Could not read this organization's tenants", helpers.APIErrorDetail(err))
		return
	}

	plan := planAssignment(environment, tenants, requested)
	if len(plan.problems) > 0 {
		resp.Diagnostics.AddAttributeError(
			path.Root("tenant_ids"),
			"Tenants cannot be added to this platform environment",
			fmt.Sprintf("Nothing was sent. %d of the %d tenants cannot join the platform environment \"%s\":\n\n- %s",
				len(plan.problems), len(requested), environment.Name, strings.Join(plan.problems, "\n- ")),
		)
		return
	}

	if len(plan.toAssign) == 0 {
		resp.SendProgress(action.InvokeProgressEvent{
			Message: fmt.Sprintf("All %d tenants are already in platform environment %s; nothing to add",
				len(plan.alreadyIn), environment.Name),
		})
		return
	}

	ids := make([]string, len(plan.toAssign))
	for i, t := range plan.toAssign {
		ids[i] = t.ID
	}

	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Adding %d tenant(s) to platform environment %s (%s)%s. This cannot be undone",
			len(ids), environment.Name, environment.ID, skippedClause(plan.alreadyIn)),
	})

	if err := a.client.AssignTenants(ctx, environmentID, &account.AssignTenantsRequest{TenantIds: ids}); err != nil {
		resp.Diagnostics.AddError(
			"Tenants could not be added to the platform environment",
			fmt.Sprintf("Jamf Account refused to add %d tenant(s) to \"%s\": %s%s", len(ids), environment.Name,
				helpers.APIErrorDetail(err), a.partialOutcome(ctx, environmentID, ids)),
		)
		return
	}

	after, err := a.client.ListTenants(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"The assignment could not be confirmed",
			fmt.Sprintf("Jamf Account accepted the request to add %d tenant(s) to \"%s\", but reading the tenants "+
				"back failed, so whether it took is unknown: %s", len(ids), environment.Name, helpers.APIErrorDetail(err)),
		)
		return
	}
	if missing := notInEnvironment(after, environmentID, ids); len(missing) > 0 {
		resp.Diagnostics.AddError(
			"Jamf Account did not report the assignment",
			fmt.Sprintf("Jamf Account accepted the request, but %d tenant(s) do not report platform environment "+
				"\"%s\" when read back: %s. Read the `jamfplatform_account_tenants` data source to see where they "+
				"stand.", len(missing), environment.Name, strings.Join(missing, ", ")),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Added %d tenant(s) to platform environment %s", len(ids), environment.Name),
	})
}

// partialOutcome reports, best effort, which of the tenants a failed write named
// are now in the platform environment, so a failure that half applied is visible.
// A failure to read the tenants adds nothing: the write's own error is the
// message that matters.
func (a *AssignTenantsAction) partialOutcome(ctx context.Context, environmentID string, ids []string) string {
	after, err := a.client.ListTenants(ctx)
	if err != nil {
		return ""
	}
	var landed []string
	for _, t := range accounttenant.InEnvironment(after, environmentID) {
		for _, id := range ids {
			if t.ID == id {
				landed = append(landed, accounttenant.Describe(t))
			}
		}
	}
	if len(landed) == 0 {
		return ""
	}
	return fmt.Sprintf(". Some of them are in the platform environment now: %s", strings.Join(landed, "; "))
}

// skippedClause names how many requested tenants were left out because they are
// already in the platform environment.
func skippedClause(alreadyIn []account.Tenant) string {
	if len(alreadyIn) == 0 {
		return ""
	}
	return fmt.Sprintf(", skipping %d already in it", len(alreadyIn))
}

// findEnvironment returns the platform environment with the given identifier, or
// nil.
func findEnvironment(environments []account.Environment, id string) *account.Environment {
	for i := range environments {
		if environments[i].ID == id {
			return &environments[i]
		}
	}
	return nil
}
