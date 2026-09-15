// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_invitation

import (
	"context"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildVPPInvitationInput projects the plan into an SDK *proclassic.VppInvitation
// for Create / Update.
//
// General scalars are emitted as the plan holds them. The four email-mode
// strings (sender_name, sender_email_address, subject, message) are emitted on
// every write, empty when null (helpers.AlwaysEmitStringPointer): the classic
// PUT merges field by field, so an omitted element would keep a value the
// config dropped and Read would echo it back as an inconsistent result (issue
// #384). Under "Send emails" the validator requires all four, so the empty
// element only ever reaches the wire under the modes that do not use them.
// This endpoint could not be probed here — the test estate holds no VPP token
// — so the rule is carried over from the five sibling classic endpoints probed
// 2026-09-06 (printers, networksegments, directorybindings, webhooks,
// patchexternalsources), on each of which an empty element is accepted and
// clears wherever the active mode does not require the field. require_login
// stays omit-when-null: a boolean's clear token cannot be assumed unprobed.
// Scope follows per-category granular ownership: only declared categories are
// emitted (see buildScope). A nil Scope omits <scope> entirely and leaves the
// server's scope untouched.
//
// NOTE: the general section is field-order sensitive on the wire — do not
// reorder the struct assignments below.
func buildVPPInvitationInput(ctx context.Context, plan VPPInvitationResourceModel) (*proclassic.VppInvitation, diag.Diagnostics) {
	var diags diag.Diagnostics

	general := &proclassic.VppInvitationGeneral{
		Name:                     helpers.OptionalStringPointer(plan.Name),
		DistributionMethod:       helpers.OptionalStringPointer(plan.DistributionMethod),
		AutoRegisterManagedUsers: helpers.OptionalBoolPointer(plan.AutoRegisterManagedUsers),
		SenderName:               helpers.AlwaysEmitStringPointer(plan.SenderName),
		SenderEmailAddress:       helpers.AlwaysEmitStringPointer(plan.SenderEmailAddress),
		Subject:                  helpers.AlwaysEmitStringPointer(plan.Subject),
		Message:                  encodedMessagePointer(plan.Message),
		RequireLogin:             helpers.OptionalBoolPointer(plan.RequireLogin),
	}
	if acctID := helpers.StringIDPtr(plan.VPPAccountID); acctID != nil {
		general.VppAccount = &proclassic.VppInvitationGeneralVppAccount{ID: acctID}
	}

	out := &proclassic.VppInvitation{General: general}

	if plan.Scope != nil {
		s, d := buildScope(ctx, plan.Scope)
		diags.Append(d...)
		out.Scope = s
	}

	return out, diags
}

// buildScope projects a declared scope model into the wire scope, emitting
// only the categories the model declares: a null category leaves its wrapper
// nil so the element is omitted entirely (members maintained in the admin UI
// survive the write), while a declared `[]` yields a non-nil empty slice
// (scope.BuildIDSlice / BuildNameSlice) whose wrapper marshals as an explicit
// empty element — the clear gesture. Create passes the raw plan, so undeclared
// categories never reach the wire; Update passes the scope.MergeUserScope
// output, whose fields are all non-null, so the full skeleton emerges
// naturally from the merge and the replace-the-whole-subtree write lands
// exactly the merged model.
func buildScope(ctx context.Context, m *scope.UserScopeModel) (*proclassic.VppInvitationScope, diag.Diagnostics) {
	var diags diag.Diagnostics
	t := m.TargetsOrZero()
	s := &proclassic.VppInvitationScope{
		AllJssUsers: helpers.OptionalBoolPointer(t.AllJssUsers),
	}

	jssUsers, d := scope.BuildIDSlice(ctx, t.JssUserIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if jssUsers != nil {
		s.JssUsers = &proclassic.VppInvitationScopeJssUsers{User: jssUsers}
	}

	jssUserGroups, d := scope.BuildIDSlice(ctx, t.JssUserGroupIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if jssUserGroups != nil {
		s.JssUserGroups = &proclassic.VppInvitationScopeJssUserGroups{UserGroup: jssUserGroups}
	}

	// limitations.user_groups: directory-service groups, NAME-keyed (populate
	// Name). The block is emitted whenever declared; the category wrapper only
	// when the category itself is declared.
	if m.Limitations != nil {
		limNames, ld := scope.BuildNameSlice(ctx, m.Limitations.DirectoryServiceUserGroupNames, func(name string) proclassic.IDName {
			n := name
			return proclassic.IDName{Name: &n}
		})
		diags.Append(ld...)
		l := &proclassic.VppInvitationScopeLimitations{}
		if limNames != nil {
			l.UserGroups = &proclassic.VppInvitationScopeLimitationsUserGroups{UserGroup: limNames}
		}
		s.Limitations = l
	}

	// exclusions: id-keyed jss_users / jss_user_groups + name-keyed user_groups.
	if m.Exclusions != nil {
		excl := &proclassic.VppInvitationScopeExclusions{}
		exclJssUsers, ed := scope.BuildIDSlice(ctx, m.Exclusions.JssUserIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
		diags.Append(ed...)
		if exclJssUsers != nil {
			excl.JssUsers = &proclassic.VppInvitationScopeExclusionsJssUsers{User: exclJssUsers}
		}
		exclJssUserGroups, ed := scope.BuildIDSlice(ctx, m.Exclusions.JssUserGroupIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
		diags.Append(ed...)
		if exclJssUserGroups != nil {
			excl.JssUserGroups = &proclassic.VppInvitationScopeExclusionsJssUserGroups{UserGroup: exclJssUserGroups}
		}
		exclDSGroups, ed := scope.BuildNameSlice(ctx, m.Exclusions.DirectoryServiceUserGroupNames, func(name string) proclassic.VppInvitationScopeExclusionsUserGroupsUserGroupItem {
			n := name
			return proclassic.VppInvitationScopeExclusionsUserGroupsUserGroupItem{Name: &n}
		})
		diags.Append(ed...)
		if exclDSGroups != nil {
			excl.UserGroups = &proclassic.VppInvitationScopeExclusionsUserGroups{UserGroup: exclDSGroups}
		}
		s.Exclusions = excl
	}

	// Omission semantics (STYLE_GUIDE.md §Scope helper): collapse to nil when
	// nothing at all is declared so the payload omits <scope> entirely.
	if s.AllJssUsers == nil && s.JssUsers == nil && s.JssUserGroups == nil &&
		s.Limitations == nil && s.Exclusions == nil {
		return nil, diags
	}
	return s, diags
}

// encodedMessagePointer form-URL-encodes the invitation email message so the
// server's form-decode of the <message> field round-trips it verbatim. The
// classic endpoint form-decodes this field (wire-probed: bare `%` not followed by
// two hex digits → HTTP 500; `+` → space; `%XX` → byte). url.QueryEscape makes
// the message survive that decode exactly — including the `%@` registration-URL
// placeholder and embedded newlines — so state (decoded on GET) matches config.
// Null encodes to an empty element (the clear gesture, see
// buildVPPInvitationInput); unknown is omitted.
func encodedMessagePointer(v types.String) *string {
	raw := helpers.AlwaysEmitStringPointer(v)
	if raw == nil {
		return nil
	}
	s := url.QueryEscape(*raw)
	return &s
}
