// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprints

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	bp "github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// BlueprintsDataSource implements the Terraform data source for listing blueprints.
type BlueprintsDataSource struct {
	client *bp.Client
}

// BlueprintsDataSourceModel defines the state for the blueprints listing data source.
type BlueprintsDataSourceModel struct {
	ID         types.String        `tfsdk:"id"`
	Search     types.String        `tfsdk:"search"`
	Blueprints []BlueprintListItem `tfsdk:"blueprints"`
}

// BlueprintListItem represents a single blueprint overview entry.
type BlueprintListItem struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Description           types.String `tfsdk:"description"`
	Created               types.String `tfsdk:"created"`
	Updated               types.String `tfsdk:"updated"`
	DeploymentState       types.String `tfsdk:"deployment_state"`
	LastDeploymentState   types.String `tfsdk:"last_deployment_state"`
	LastDeploymentStarted types.String `tfsdk:"last_deployment_started"`
}
