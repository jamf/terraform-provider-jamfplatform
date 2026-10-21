// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignDirectoryBindingResourceModel populates a resource model from a
// DirectoryBinding response. state.ID is only overwritten when the API ID is
// non-nil so a transient GET that drops the ID does not clobber the value
// persisted from Create.
//
// state.Password is `WriteOnly` — the framework excludes it from state
// regardless of what we write, so we do not need to touch it. The Jamf Pro
// classic GET response never echoes the plaintext anyway, only the redacted
// `password_sha256` sentinel which carries no drift-detection signal and is
// no longer surfaced. state.PasswordWoVersion is preserved verbatim by
// the framework (regular Optional Int64, not WriteOnly).
//
// The empty `<powerbroker_identity_services/>` wire element does not surface
// in state — the schema exposes no nested block for PowerBroker since it
// carries no per-type fields. The `type` field on its own conveys the
// PowerBroker identity.
//
// `domain`, `username` and `computer_ou` are reconciled rather than copied:
// the input builder always emits them, so an attribute the config omits is
// sent empty and echoed back as "", which
// helpers.ReconcileOptionalStringPointer folds to null against the incoming
// model (plan on write, prior state on refresh) while keeping an explicit ""
// the user configured. The free-text fields of the nested per-type blocks are
// reconciled the same way against the incoming block, because Jamf Pro echoes
// each one sent as "" as an empty element (#445).
func assignDirectoryBindingResourceModel(state *DirectoryBindingResourceModel, b *proclassic.DirectoryBinding) diag.Diagnostics {
	var diags diag.Diagnostics
	if b == nil {
		return diags
	}
	if b.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(b.ID)
	}
	state.Name = helpers.StringPointerValueOrNullTrimmed(b.Name, state.Name)
	state.Type = helpers.StringPointerValueOrNull(b.Type)
	state.Domain = helpers.ReconcileOptionalStringPointerTrimmed(b.Domain, state.Domain)
	state.Username = helpers.ReconcileOptionalStringPointerTrimmed(b.Username, state.Username)
	state.ComputerOU = helpers.ReconcileOptionalStringPointerTrimmed(b.ComputerOu, state.ComputerOU)
	state.Priority = helpers.Int64FromIntPtr(b.Priority)

	state.ActiveDirectory = assignActiveDirectoryModel(b.ActiveDirectory, state.ActiveDirectory)
	state.OpenDirectory = assignOpenDirectoryModel(b.OpenDirectory)
	state.Admitmac = assignAdmitmacModel(b.Admitmac, state.Admitmac)
	state.Centrify = assignCentrifyModel(b.Centrify, state.Centrify)

	return diags
}

// assignDirectoryBindingDataSourceModel populates a data source model from a
// DirectoryBinding response. Symmetric with the resource builder, minus the
// `password` field — data source is read-only and the classic GET never
// echoes the plaintext anyway.
func assignDirectoryBindingDataSourceModel(state *DirectoryBindingDataSourceModel, b *proclassic.DirectoryBinding) diag.Diagnostics {
	var diags diag.Diagnostics
	if b == nil {
		return diags
	}
	if b.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(b.ID)
	}
	state.Name = helpers.StringPointerValueOrNullTrimmed(b.Name, state.Name)
	state.Type = helpers.StringPointerValueOrNull(b.Type)
	state.Domain = helpers.StringPointerValueOrNull(b.Domain)
	state.Username = helpers.StringPointerValueOrNull(b.Username)
	state.ComputerOU = helpers.StringPointerValueOrNull(b.ComputerOu)
	state.Priority = helpers.Int64FromIntPtr(b.Priority)

	state.ActiveDirectory = assignActiveDirectoryModel(b.ActiveDirectory, nil)
	state.OpenDirectory = assignOpenDirectoryModel(b.OpenDirectory)
	state.Admitmac = assignAdmitmacModel(b.Admitmac, nil)
	state.Centrify = assignCentrifyModel(b.Centrify, nil)

	return diags
}

// assignActiveDirectoryModel decodes the nested SDK block into the TF
// model, or returns nil when the API did not include the block. A nil
// return tells the framework to omit the SingleNestedAttribute from state
// entirely rather than surfacing it as a struct full of nulls.
//
// prior is the incoming block (plan on write, prior state on refresh, nil for
// a data source or an import). Each free-text field is reconciled against it
// with helpers.ReconcileOptionalStringPointer, so an authored "" that Jamf Pro
// echoes as an empty element stays "" rather than collapsing to null.
func assignActiveDirectoryModel(a *proclassic.DirectoryBindingActiveDirectory, prior *directoryBindingActiveDirectoryModel) *directoryBindingActiveDirectoryModel {
	if a == nil {
		return nil
	}
	if prior == nil {
		prior = &directoryBindingActiveDirectoryModel{}
	}
	return &directoryBindingActiveDirectoryModel{
		Forest:                  helpers.ReconcileOptionalStringPointerTrimmed(a.Forest, prior.Forest),
		CreateMobileAccount:     helpers.BoolPointerValueOrNull(a.CacheLastUser),
		RequireConfirmation:     helpers.BoolPointerValueOrNull(a.RequireConfirmation),
		ForceLocalHomeDirectory: helpers.BoolPointerValueOrNull(a.LocalHome),
		UseUncPath:              helpers.BoolPointerValueOrNull(a.UseUncPath),
		NetworkProtocol:         helpers.ReconcileOptionalStringPointerTrimmed(a.MountStyle, prior.NetworkProtocol),
		DefaultShell:            helpers.ReconcileOptionalStringPointerTrimmed(a.DefaultShell, prior.DefaultShell),
		UIDAttributeMapping:     helpers.ReconcileOptionalStringPointerTrimmed(a.Uid, prior.UIDAttributeMapping),
		UserGIDAttributeMapping: helpers.ReconcileOptionalStringPointerTrimmed(a.UserGid, prior.UserGIDAttributeMapping),
		GIDAttributeMapping:     helpers.ReconcileOptionalStringPointerTrimmed(a.Gid, prior.GIDAttributeMapping),
		MultipleDomains:         helpers.BoolPointerValueOrNull(a.MultipleDomains),
		PreferredDomain:         helpers.ReconcileOptionalStringPointerTrimmed(a.PreferredDomain, prior.PreferredDomain),
		AdminGroups:             helpers.ReconcileOptionalStringPointerTrimmed(a.AdminGroups, prior.AdminGroups),
	}
}

// assignOpenDirectoryModel decodes the nested SDK block into the TF model.
func assignOpenDirectoryModel(o *proclassic.DirectoryBindingOpenDirectory) *directoryBindingOpenDirectoryModel {
	if o == nil {
		return nil
	}
	return &directoryBindingOpenDirectoryModel{
		EncryptUsingSSL:      helpers.BoolPointerValueOrNull(o.EncryptUsingSsl),
		PerformSecureBind:    helpers.BoolPointerValueOrNull(o.PerformSecureBind),
		UseForAuthentication: helpers.BoolPointerValueOrNull(o.UseForAuthentication),
		UseForContacts:       helpers.BoolPointerValueOrNull(o.UseForContacts),
	}
}

// assignAdmitmacModel decodes the nested SDK block into the TF model, with
// the same prior-block reconciliation as assignActiveDirectoryModel.
func assignAdmitmacModel(a *proclassic.DirectoryBindingAdmitmac, prior *directoryBindingAdmitmacModel) *directoryBindingAdmitmacModel {
	if a == nil {
		return nil
	}
	if prior == nil {
		prior = &directoryBindingAdmitmacModel{}
	}
	return &directoryBindingAdmitmacModel{
		RequireConfirmation:     helpers.BoolPointerValueOrNull(a.RequireConfirmation),
		HomeLocation:            helpers.ReconcileOptionalStringPointerTrimmed(a.LocalHome, prior.HomeLocation),
		NetworkProtocol:         helpers.ReconcileOptionalStringPointerTrimmed(a.MountStyle, prior.NetworkProtocol),
		DefaultShell:            helpers.ReconcileOptionalStringPointerTrimmed(a.DefaultShell, prior.DefaultShell),
		MountNetworkHome:        helpers.BoolPointerValueOrNull(a.MountNetworkHome),
		PlaceHomeFolders:        helpers.ReconcileOptionalStringPointerTrimmed(a.PlaceHomeFolders, prior.PlaceHomeFolders),
		UIDAttributeMapping:     helpers.ReconcileOptionalStringPointerTrimmed(a.Uid, prior.UIDAttributeMapping),
		UserGIDAttributeMapping: helpers.ReconcileOptionalStringPointerTrimmed(a.UserGid, prior.UserGIDAttributeMapping),
		GIDAttributeMapping:     helpers.ReconcileOptionalStringPointerTrimmed(a.Gid, prior.GIDAttributeMapping),
		AdminGroup:              helpers.ReconcileOptionalStringPointerTrimmed(a.AdminGroup, prior.AdminGroup),
		CachedCredentials:       helpers.Int64FromIntPtr(a.CachedCredentials),
		AddUserToLocal:          helpers.BoolPointerValueOrNull(a.AddUserToLocal),
		UsersOU:                 helpers.ReconcileOptionalStringPointerTrimmed(a.UsersOu, prior.UsersOU),
		GroupsOU:                helpers.ReconcileOptionalStringPointerTrimmed(a.GroupsOu, prior.GroupsOU),
		PrintersOU:              helpers.ReconcileOptionalStringPointerTrimmed(a.PrintersOu, prior.PrintersOU),
		SharedFoldersOU:         helpers.ReconcileOptionalStringPointerTrimmed(a.SharedFoldersOu, prior.SharedFoldersOU),
	}
}

// assignCentrifyModel decodes the nested SDK block into the TF model, with
// the same prior-block reconciliation as assignActiveDirectoryModel.
func assignCentrifyModel(c *proclassic.DirectoryBindingCentrify, prior *directoryBindingCentrifyModel) *directoryBindingCentrifyModel {
	if c == nil {
		return nil
	}
	if prior == nil {
		prior = &directoryBindingCentrifyModel{}
	}
	return &directoryBindingCentrifyModel{
		WorkstationMode:       helpers.BoolPointerValueOrNull(c.WorkstationMode),
		OverwriteExisting:     helpers.BoolPointerValueOrNull(c.OverwriteExisting),
		UpdatePAM:             helpers.BoolPointerValueOrNull(c.UpdatePAM),
		Zone:                  helpers.ReconcileOptionalStringPointerTrimmed(c.Zone, prior.Zone),
		PreferredDomainServer: helpers.ReconcileOptionalStringPointerTrimmed(c.PreferredDomainServer, prior.PreferredDomainServer),
	}
}
