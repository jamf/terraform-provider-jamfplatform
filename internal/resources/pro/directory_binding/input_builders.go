// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// mapTypeToWire returns the wire `type` value to send on Create / Update.
// Wraps mapType (helpers.go) with the TF Bool/String-aware nil semantics
// of helpers.OptionalStringPointer — null / unknown stays nil.
func mapTypeToWire(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	out := mapType(v.ValueString())
	return &out
}

// buildDirectoryBindingInput converts the Terraform plan model into the SDK
// DirectoryBinding payload used for both Create and Update.
//
// Three non-default behaviours, the first two confirmed by the §13.2 audit
// and the third wire-probed 2026-09-06 on Jamf Pro 11.31.1 (issue #384):
//
//   - Plaintext `Password` is sourced from req.Config (it's a WriteOnly
//     attribute, so req.Plan exposes it as null). The caller of this builder
//     decides whether to thread a non-nil plaintext through based on the
//     `password_wo_version` rotation trigger — when the trigger is unchanged
//     the caller passes nil so this builder omits `<password/>` on the wire
//     and Classic's partial-merge semantics retain the server's stored
//     value. The server-side `password_sha256` sentinel is no longer surfaced
//     in state (the field returns the literal 20-asterisk redaction string,
//     not a real hash, so it carries no drift-detection signal).
//
//   - The empty `<powerbroker_identity_services/>` element is emitted by
//     attaching a non-nil empty struct whenever `type =
//     "PowerBroker Identity Services"`. The PowerBroker variant has no
//     per-type fields so the TF schema exposes no nested block — the input
//     builder synthesises the SDK struct based purely on `type`.
//
//   - `domain`, `username` and `computer_ou` are emitted on every write,
//     empty when null (helpers.AlwaysEmitStringPointer). The classic
//     /directorybindings PUT merges field by field: an omitted element keeps
//     the stored value, an empty one clears it — accepted for all three on an
//     Active Directory binding. Omitting them would leave a value the user
//     removed from config on the server, and Read would echo it back as an
//     inconsistent result. The state builder reconciles the echoed "" to null.
//
// `ID` is omitted on write — Create uses path id="0" and Update derives ID
// from state.
func buildDirectoryBindingInput(plan DirectoryBindingResourceModel, password *string) *proclassic.DirectoryBinding {
	in := &proclassic.DirectoryBinding{
		Name:       helpers.OptionalStringPointer(plan.Name),
		Type:       mapTypeToWire(plan.Type),
		Domain:     helpers.AlwaysEmitStringPointer(plan.Domain),
		Username:   helpers.AlwaysEmitStringPointer(plan.Username),
		Password:   password,
		ComputerOu: helpers.AlwaysEmitStringPointer(plan.ComputerOU),
		Priority:   helpers.OptionalInt64Pointer(plan.Priority),
	}

	switch plan.Type.ValueString() {
	case typeActiveDirectory:
		in.ActiveDirectory = buildActiveDirectoryInput(plan.ActiveDirectory)
	case typeOpenDirectory:
		in.OpenDirectory = buildOpenDirectoryInput(plan.OpenDirectory)
	case typeADmitMac:
		in.Admitmac = buildAdmitmacInput(plan.Admitmac)
	case typeCentrify:
		in.Centrify = buildCentrifyInput(plan.Centrify)
	case typePowerBroker:
		// Empty element, but it must round-trip. The SDK encodes a non-nil
		// empty struct as `<powerbroker_identity_services/>`; a nil pointer
		// would omit the element entirely and the server would not classify
		// the binding as PowerBroker.
		in.PowerbrokerIdentityServices = &proclassic.DirectoryBindingPowerbrokerIdentityServices{}
	}

	return in
}

// buildActiveDirectoryInput converts the TF Active Directory nested block
// into the SDK shape. Returns nil when the user omits the block — the SDK's
// omitempty drops `<active_directory>` from the wire so the server preserves
// stored values under Classic's partial-merge semantics.
func buildActiveDirectoryInput(m *directoryBindingActiveDirectoryModel) *proclassic.DirectoryBindingActiveDirectory {
	if m == nil {
		return nil
	}
	return &proclassic.DirectoryBindingActiveDirectory{
		Forest:              helpers.OptionalStringPointer(m.Forest),
		CacheLastUser:       helpers.OptionalBoolPointer(m.CreateMobileAccount),
		RequireConfirmation: helpers.OptionalBoolPointer(m.RequireConfirmation),
		LocalHome:           helpers.OptionalBoolPointer(m.ForceLocalHomeDirectory),
		UseUncPath:          helpers.OptionalBoolPointer(m.UseUncPath),
		MountStyle:          helpers.OptionalStringPointer(m.NetworkProtocol),
		DefaultShell:        helpers.OptionalStringPointer(m.DefaultShell),
		Uid:                 helpers.OptionalStringPointer(m.UIDAttributeMapping),
		UserGid:             helpers.OptionalStringPointer(m.UserGIDAttributeMapping),
		Gid:                 helpers.OptionalStringPointer(m.GIDAttributeMapping),
		MultipleDomains:     helpers.OptionalBoolPointer(m.MultipleDomains),
		PreferredDomain:     helpers.OptionalStringPointer(m.PreferredDomain),
		AdminGroups:         helpers.OptionalStringPointer(m.AdminGroups),
	}
}

// buildOpenDirectoryInput converts the TF Open Directory nested block into
// the SDK shape.
func buildOpenDirectoryInput(m *directoryBindingOpenDirectoryModel) *proclassic.DirectoryBindingOpenDirectory {
	if m == nil {
		return nil
	}
	return &proclassic.DirectoryBindingOpenDirectory{
		EncryptUsingSsl:      helpers.OptionalBoolPointer(m.EncryptUsingSSL),
		PerformSecureBind:    helpers.OptionalBoolPointer(m.PerformSecureBind),
		UseForAuthentication: helpers.OptionalBoolPointer(m.UseForAuthentication),
		UseForContacts:       helpers.OptionalBoolPointer(m.UseForContacts),
	}
}

// buildAdmitmacInput converts the TF ADmitMac nested block into the SDK
// shape. `local_home` is a string here (e.g. "Local"), not a bool — see
// model_types.go.
func buildAdmitmacInput(m *directoryBindingAdmitmacModel) *proclassic.DirectoryBindingAdmitmac {
	if m == nil {
		return nil
	}
	return &proclassic.DirectoryBindingAdmitmac{
		RequireConfirmation: helpers.OptionalBoolPointer(m.RequireConfirmation),
		LocalHome:           helpers.OptionalStringPointer(m.HomeLocation),
		MountStyle:          helpers.OptionalStringPointer(m.NetworkProtocol),
		DefaultShell:        helpers.OptionalStringPointer(m.DefaultShell),
		MountNetworkHome:    helpers.OptionalBoolPointer(m.MountNetworkHome),
		PlaceHomeFolders:    helpers.OptionalStringPointer(m.PlaceHomeFolders),
		Uid:                 helpers.OptionalStringPointer(m.UIDAttributeMapping),
		UserGid:             helpers.OptionalStringPointer(m.UserGIDAttributeMapping),
		Gid:                 helpers.OptionalStringPointer(m.GIDAttributeMapping),
		AdminGroup:          helpers.OptionalStringPointer(m.AdminGroup),
		CachedCredentials:   helpers.OptionalInt64Pointer(m.CachedCredentials),
		AddUserToLocal:      helpers.OptionalBoolPointer(m.AddUserToLocal),
		UsersOu:             helpers.OptionalStringPointer(m.UsersOU),
		GroupsOu:            helpers.OptionalStringPointer(m.GroupsOU),
		PrintersOu:          helpers.OptionalStringPointer(m.PrintersOU),
		SharedFoldersOu:     helpers.OptionalStringPointer(m.SharedFoldersOU),
	}
}

// buildCentrifyInput converts the TF Centrify nested block into the SDK
// shape.
func buildCentrifyInput(m *directoryBindingCentrifyModel) *proclassic.DirectoryBindingCentrify {
	if m == nil {
		return nil
	}
	return &proclassic.DirectoryBindingCentrify{
		WorkstationMode:       helpers.OptionalBoolPointer(m.WorkstationMode),
		OverwriteExisting:     helpers.OptionalBoolPointer(m.OverwriteExisting),
		UpdatePAM:             helpers.OptionalBoolPointer(m.UpdatePAM),
		Zone:                  helpers.OptionalStringPointer(m.Zone),
		PreferredDomainServer: helpers.OptionalStringPointer(m.PreferredDomainServer),
	}
}
