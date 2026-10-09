// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_json_web_token_configuration

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// JSONWebTokenConfigurationResourceModel is the Terraform resource model for a
// Jamf Pro JSON Web Token configuration. The wire envelope is flat — five
// scalars, no <general> wrapper.
//
//   - `EncryptionKey` is `WriteOnly`: the plaintext key is sent on writes but
//     never persisted in state (Jamf Pro never returns it).
//     `EncryptionKeyWoVersion` is the rotation trigger — bump it to force the
//     next Update to re-send the current `EncryptionKey`.
//   - `Enabled` is the inverse of the server's `disabled` flag; the inversion
//     lives entirely in the input/state builders.
type JSONWebTokenConfigurationResourceModel struct {
	ID                     types.String           `tfsdk:"id"`
	Name                   types.String           `tfsdk:"name"`
	EncryptionKey          types.String           `tfsdk:"encryption_key_wo"`
	EncryptionKeyWoVersion types.Int64            `tfsdk:"encryption_key_wo_version"`
	TokenExpiry            types.Int64            `tfsdk:"token_expiry"`
	Enabled                types.Bool             `tfsdk:"enabled"`
	Timeouts               resourceTimeouts.Value `tfsdk:"timeouts"`
}

// JSONWebTokenConfigurationDataSourceModel is the flat read-only data source
// projection. Selects by `id` or exact `name` (ExactlyOneOf). The encryption
// key is omitted (Jamf Pro never returns it).
type JSONWebTokenConfigurationDataSourceModel struct {
	ID          types.String             `tfsdk:"id"`
	Name        types.String             `tfsdk:"name"`
	TokenExpiry types.Int64              `tfsdk:"token_expiry"`
	Enabled     types.Bool               `tfsdk:"enabled"`
	Timeouts    datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// jsonWebTokenConfigurationIdentityModel represents the identity object for the
// resource and list results.
type jsonWebTokenConfigurationIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// JSONWebTokenConfigurationListResourceModel represents the config model for
// list queries. The classic endpoint has no server-side filtering — the filter
// shape is the shared client-side substring block.
type JSONWebTokenConfigurationListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
