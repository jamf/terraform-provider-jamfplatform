// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// The webhook auth/event fields are gated server-side, but the server enforces
// them *silently* (it stores "" for an inactive auth field rather than 409ing —
// WEBHOOK_SPIKE.md §5 invariant 4), which would otherwise re-diff every plan.
// These plan-time validators surface the rules as config errors before apply.
// `smart_group_id` is the exception — the server genuinely 409s it on a
// non-smart event (invariant 5) — but the same plan-time guard gives a better
// message than a raw Conflict.
//
// Each validator's decision is a pure func(WebhookResourceModel) *ruleViolation
// so it can be unit-tested without constructing a tfsdk.Config.

// ruleViolation is a self-contained cross-field diagnostic.
type ruleViolation struct {
	attr    string
	summary string
	detail  string
}

func (rv ruleViolation) add(resp *resource.ValidateConfigResponse) {
	resp.Diagnostics.AddAttributeError(path.Root(rv.attr), rv.summary, rv.detail)
}

// checkUsernameRequiresBasic enforces username ⇒ authentication_type=BASIC.
func checkUsernameRequiresBasic(d WebhookResourceModel) *ruleViolation {
	if !helpers.IsConfiguredValue(d.Username) || !helpers.IsConfiguredValue(d.AuthenticationType) {
		return nil
	}
	if d.AuthenticationType.ValueString() == authTypeBasic {
		return nil
	}
	return &ruleViolation{
		attr:    "username",
		summary: "username requires BASIC authentication",
		detail:  fmt.Sprintf("`username` is only used by BASIC authentication, but `authentication_type` is %q. Remove `username` or set `authentication_type = \"BASIC\"`.", d.AuthenticationType.ValueString()),
	}
}

// checkPasswordRequiresBasicOrHash enforces password ⇒ BASIC or HASH_SIGNATURE.
func checkPasswordRequiresBasicOrHash(d WebhookResourceModel) *ruleViolation {
	if !helpers.IsConfiguredValue(d.Password) || !helpers.IsConfiguredValue(d.AuthenticationType) {
		return nil
	}
	switch d.AuthenticationType.ValueString() {
	case authTypeBasic, authTypeHashSignature:
		return nil
	default:
		return &ruleViolation{
			attr:    "password",
			summary: "password requires BASIC or HASH_SIGNATURE authentication",
			detail:  fmt.Sprintf("`password` is the BASIC password or the HASH_SIGNATURE signing secret, but `authentication_type` is %q. Remove `password` or set a compatible `authentication_type`.", d.AuthenticationType.ValueString()),
		}
	}
}

// checkHeaderRequiresHeaderAuth enforces header ⇒ authentication_type=HEADER.
func checkHeaderRequiresHeaderAuth(d WebhookResourceModel) *ruleViolation {
	if !helpers.IsConfiguredValue(d.Header) || !helpers.IsConfiguredValue(d.AuthenticationType) {
		return nil
	}
	if d.AuthenticationType.ValueString() == authTypeHeader {
		return nil
	}
	return &ruleViolation{
		attr:    "header",
		summary: "header requires HEADER authentication",
		detail:  fmt.Sprintf("`header` is only used by HEADER authentication, but `authentication_type` is %q. Remove `header` or set `authentication_type = \"HEADER\"`.", d.AuthenticationType.ValueString()),
	}
}

// checkBasicRequiresUsername enforces authentication_type=BASIC ⇒ username.
// The server refuses a BASIC webhook without one — `409 Username is required`,
// wire-probed 2026-09-06 — and, because the input builder sends `username`
// as configured under BASIC, a config that drops it would otherwise omit the
// element, the classic merge would retain the old value, and Read would echo
// it back as an inconsistent result. An empty username counts as missing: the
// server answers it with the same 409 (wire-probed 2026-10-01). An unknown
// username is skipped: it will be known at apply.
func checkBasicRequiresUsername(d WebhookResourceModel) *ruleViolation {
	if !helpers.IsConfiguredValue(d.AuthenticationType) || d.AuthenticationType.ValueString() != authTypeBasic {
		return nil
	}
	if d.Username.IsUnknown() || (!d.Username.IsNull() && d.Username.ValueString() != "") {
		return nil
	}
	return &ruleViolation{
		attr:    "username",
		summary: "username required for BASIC authentication",
		detail:  "`authentication_type` is \"BASIC\", which requires `username`. Set `username` (and `password`), or change `authentication_type`.",
	}
}

// checkHeaderAuthRequiresHeader enforces authentication_type=HEADER ⇒ header.
// The server refuses a HEADER webhook without one — `409 INVALID_REQUIRED`,
// wire-probed 2026-09-06 — with the same retained-on-omission consequence
// described on checkBasicRequiresUsername. An empty header counts as missing,
// for the same reason.
func checkHeaderAuthRequiresHeader(d WebhookResourceModel) *ruleViolation {
	if !helpers.IsConfiguredValue(d.AuthenticationType) || d.AuthenticationType.ValueString() != authTypeHeader {
		return nil
	}
	if d.Header.IsUnknown() || (!d.Header.IsNull() && d.Header.ValueString() != "") {
		return nil
	}
	return &ruleViolation{
		attr:    "header",
		summary: "header required for HEADER authentication",
		detail:  "`authentication_type` is \"HEADER\", which requires `header`. Set `header` to a JSON object of header name/value pairs, or change `authentication_type`.",
	}
}

// checkSmartGroupIDRequiresSmartEvent enforces smart_group_id ⇒ a SmartGroup* event.
func checkSmartGroupIDRequiresSmartEvent(d WebhookResourceModel) *ruleViolation {
	if !helpers.IsConfiguredValue(d.SmartGroupID) || !helpers.IsConfiguredValue(d.Event) {
		return nil
	}
	if isSmartGroupEvent(d.Event.ValueString()) {
		return nil
	}
	return &ruleViolation{
		attr:    "smart_group_id",
		summary: "smart_group_id requires a SmartGroup event",
		detail:  fmt.Sprintf("`smart_group_id` is only valid for the SmartGroupComputerMembershipChange, SmartGroupMobileDeviceMembershipChange, and SmartGroupUserMembershipChange events, but `event` is %q. Remove `smart_group_id` or change `event`.", d.Event.ValueString()),
	}
}

// ---- ConfigValidator wrappers --------------------------------------------------

type usernameRequiresBasicValidator struct{}

func (usernameRequiresBasicValidator) Description(context.Context) string {
	return "`username` may only be set when `authentication_type` is \"BASIC\""
}
func (v usernameRequiresBasicValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (usernameRequiresBasicValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data WebhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if rv := checkUsernameRequiresBasic(data); rv != nil {
		rv.add(resp)
	}
}

type passwordRequiresBasicOrHashValidator struct{}

func (passwordRequiresBasicOrHashValidator) Description(context.Context) string {
	return "`password` may only be set when `authentication_type` is \"BASIC\" or \"HASH_SIGNATURE\""
}
func (v passwordRequiresBasicOrHashValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (passwordRequiresBasicOrHashValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data WebhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if rv := checkPasswordRequiresBasicOrHash(data); rv != nil {
		rv.add(resp)
	}
}

type headerRequiresHeaderAuthValidator struct{}

func (headerRequiresHeaderAuthValidator) Description(context.Context) string {
	return "`header` may only be set when `authentication_type` is \"HEADER\""
}
func (v headerRequiresHeaderAuthValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (headerRequiresHeaderAuthValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data WebhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if rv := checkHeaderRequiresHeaderAuth(data); rv != nil {
		rv.add(resp)
	}
}

type basicRequiresUsernameValidator struct{}

func (basicRequiresUsernameValidator) Description(context.Context) string {
	return "`username` is required when `authentication_type` is \"BASIC\""
}
func (v basicRequiresUsernameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (basicRequiresUsernameValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data WebhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if rv := checkBasicRequiresUsername(data); rv != nil {
		rv.add(resp)
	}
}

type headerAuthRequiresHeaderValidator struct{}

func (headerAuthRequiresHeaderValidator) Description(context.Context) string {
	return "`header` is required when `authentication_type` is \"HEADER\""
}
func (v headerAuthRequiresHeaderValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (headerAuthRequiresHeaderValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data WebhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if rv := checkHeaderAuthRequiresHeader(data); rv != nil {
		rv.add(resp)
	}
}

type smartGroupIDRequiresSmartEventValidator struct{}

func (smartGroupIDRequiresSmartEventValidator) Description(context.Context) string {
	return "`smart_group_id` may only be set when `event` is a SmartGroup membership-change event"
}
func (v smartGroupIDRequiresSmartEventValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (smartGroupIDRequiresSmartEventValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data WebhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if rv := checkSmartGroupIDRequiresSmartEvent(data); rv != nil {
		rv.add(resp)
	}
}

// Compile-time interface assertions.
var (
	_ resource.ConfigValidator = usernameRequiresBasicValidator{}
	_ resource.ConfigValidator = passwordRequiresBasicOrHashValidator{}
	_ resource.ConfigValidator = headerRequiresHeaderAuthValidator{}
	_ resource.ConfigValidator = basicRequiresUsernameValidator{}
	_ resource.ConfigValidator = headerAuthRequiresHeaderValidator{}
	_ resource.ConfigValidator = smartGroupIDRequiresSmartEventValidator{}
)
