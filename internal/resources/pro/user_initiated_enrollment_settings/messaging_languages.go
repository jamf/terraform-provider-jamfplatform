// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"context"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// languageNamesByCode fetches the live language-code → display-name map from
// the language-codes endpoint. Used both to validate a planned language-code key
// (ModifyPlan) and to resolve the name sent when first adding a language
// (reconcile create path). Names are NOT unique (e.g. zh-Hant and zh-tw both map
// to "Chinese, Traditional"), so the code is the key.
func (r *UserInitiatedEnrollmentSettingsResource) languageNamesByCode(ctx context.Context) (map[string]string, error) {
	codes, err := r.client.ListEnrollmentLanguageCodesV3(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(codes))
	for _, c := range codes {
		out[c.Value] = c.Name
	}
	return out, nil
}

// validateMessagingLanguageKeys checks every map key (a language code) against
// the live set Jamf Pro recognises, emitting a plan-time error for any unknown
// code. No-op when the collection is unmanaged (null/unknown) or the client is
// not configured (bare `terraform validate`). Read-only.
func (r *UserInitiatedEnrollmentSettingsResource) validateMessagingLanguageKeys(
	ctx context.Context,
	languages types.Map,
	diags *diag.Diagnostics,
) {
	if r.client == nil || languages.IsNull() || languages.IsUnknown() {
		return
	}

	namesByCode, err := r.languageNamesByCode(ctx)
	if err != nil {
		diags.AddError("Error reading Jamf Pro enrollment language codes", helpers.APIErrorDetail(err))
		return
	}

	for code := range languages.Elements() {
		if _, ok := namesByCode[code]; !ok {
			diags.AddError(
				"Invalid messaging_languages language code",
				"language code \""+code+"\" is not a language code recognised by Jamf Pro. Use a valid ISO 639-1 code such as en, fr or de.",
			)
		}
	}
}

// reconcileMessagingLanguages drives the tenant's per-language enrollment
// messaging to the planned map (keyed by language code). It runs OUTSIDE the /v4
// enrollment write lock (the /v3 endpoints are a separate backing store, like
// Access Groups).
//
// The /v3 PUT is a full-replace upsert: PUT to an unconfigured code creates it,
// and any omitted field is cleared. To honour the Optional+Computed schema, the
// write body is built read-merge: the current server object (or the English
// language, when first adding a code) is the base, and only the fields the user
// declared are overlaid. When the planned map is null the collection is left
// untouched. The built-in English language is never deleted.
func (r *UserInitiatedEnrollmentSettingsResource) reconcileMessagingLanguages(
	ctx context.Context,
	plan *UserInitiatedEnrollmentSettingsResourceModel,
	diags *diag.Diagnostics,
) bool {
	// Unmanaged: user did not author the messaging_languages collection.
	if plan.MessagingLanguages.IsNull() || plan.MessagingLanguages.IsUnknown() {
		return true
	}

	var planned map[string]messagingLanguageModel
	diags.Append(plan.MessagingLanguages.ElementsAs(ctx, &planned, false)...)
	if diags.HasError() {
		return false
	}

	current, err := r.client.ListEnrollmentLanguagesV3(ctx, nil)
	if err != nil {
		diags.AddError("Error reading Jamf Pro enrollment languages", helpers.APIErrorDetail(err))
		return false
	}
	currentByCode := make(map[string]pro.EnrollmentProcessTextObject, len(current))
	for _, c := range current {
		currentByCode[pointerString(c.LanguageCode)] = c
	}

	// English seed for newly-added languages (mirrors the UI "Add Language"
	// pre-fill). English always exists; fall back to a GET if somehow absent.
	englishSeed, ok := currentByCode[defaultLanguageCode]
	if !ok {
		got, gErr := r.client.GetEnrollmentLanguageV3(ctx, defaultLanguageCode)
		if gErr != nil {
			diags.AddError("Error reading Jamf Pro English enrollment language", helpers.APIErrorDetail(gErr))
			return false
		}
		if got != nil {
			englishSeed = *got
		}
	}

	// Resolve display names only when adding a code not already configured.
	var namesByCode map[string]string
	for code := range planned {
		if _, exists := currentByCode[code]; !exists {
			namesByCode, err = r.languageNamesByCode(ctx)
			if err != nil {
				diags.AddError("Error reading Jamf Pro enrollment language codes", helpers.APIErrorDetail(err))
				return false
			}
			break
		}
	}

	for _, op := range planMessagingLanguageReconcile(planned, current, englishSeed, namesByCode) {
		switch op.Action {
		case messagingLanguageUpsert:
			if _, err := r.client.UpdateEnrollmentLanguageV3(ctx, op.Code, op.Body); err != nil {
				diags.AddError("Error writing Jamf Pro enrollment language \""+op.Code+"\"", helpers.APIErrorDetail(err))
				return false
			}
		case messagingLanguageDelete:
			if err := r.client.DeleteEnrollmentLanguageV3(ctx, op.Code); err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				diags.AddError("Error deleting Jamf Pro enrollment language \""+op.Code+"\"", helpers.APIErrorDetail(err))
				return false
			}
		}
	}

	return true
}

// messagingLanguageAction enumerates the reconcile operations for one language.
type messagingLanguageAction int

const (
	// messagingLanguageUpsert is the full-replace PUT (creates an unconfigured
	// code, updates an existing one).
	messagingLanguageUpsert messagingLanguageAction = iota
	messagingLanguageDelete
)

// messagingLanguageOp is a single reconcile step: an upsert carrying the full
// PUT body, or a delete carrying just the code.
type messagingLanguageOp struct {
	Action messagingLanguageAction
	Code   string
	Body   *pro.EnrollmentProcessTextObject
}

// planMessagingLanguageReconcile computes the upsert/delete operations to drive
// the tenant's configured languages (current) to the planned map, keyed by
// language code.
//
// Rules:
//   - A planned code already present is upserted only when its read-merged body
//     differs from the current server object (no-op otherwise).
//   - A planned code not present is upserted with a body seeded from English
//     (englishSeed) and the resolved display name (namesByCode).
//   - A current code the plan no longer references is deleted, EXCEPT the
//     built-in English language (defaultLanguageCode), which is never deleted.
func planMessagingLanguageReconcile(
	planned map[string]messagingLanguageModel,
	current []pro.EnrollmentProcessTextObject,
	englishSeed pro.EnrollmentProcessTextObject,
	namesByCode map[string]string,
) []messagingLanguageOp {
	currentByCode := make(map[string]pro.EnrollmentProcessTextObject, len(current))
	for _, c := range current {
		currentByCode[pointerString(c.LanguageCode)] = c
	}

	var ops []messagingLanguageOp
	for code, m := range planned {
		seed, exists := currentByCode[code]
		name := pointerString(seed.Name)
		if !exists {
			seed = englishSeed
			name = namesByCode[code]
		}

		body := buildMessagingLanguageInput(seed, m, code, name)
		if exists && reflect.DeepEqual(*body, seed) {
			continue
		}
		ops = append(ops, messagingLanguageOp{Action: messagingLanguageUpsert, Code: code, Body: body})
	}

	for _, c := range current {
		code := pointerString(c.LanguageCode)
		if code == "" || code == defaultLanguageCode {
			continue
		}
		if _, ok := planned[code]; ok {
			continue
		}
		ops = append(ops, messagingLanguageOp{Action: messagingLanguageDelete, Code: code})
	}

	return ops
}

// buildMessagingLanguageInput builds a full-replace PUT body from a seed object
// (the current server language, or English when first adding a code) overlaid
// with the user-declared fields. languageCode and name are set explicitly (the
// code is the map key); the 38 text fields are overlaid only when the planned
// value is known, so unset fields keep the seed's value (the read-merge
// behaviour). The seed is never mutated — only the returned copy's pointer fields
// are reassigned.
func buildMessagingLanguageInput(seed pro.EnrollmentProcessTextObject, m messagingLanguageModel, code, name string) *pro.EnrollmentProcessTextObject {
	body := seed
	body.LanguageCode = &code
	body.Name = &name

	overlayString(&body.Title, m.PageTitle)

	overlayString(&body.LoginDescription, m.LoginPageText)
	overlayString(&body.Username, m.UsernameText)
	overlayString(&body.Password, m.PasswordText)
	overlayString(&body.LoginButton, m.LoginButtonText)

	overlayString(&body.DeviceClassDescription, m.DeviceOwnershipPageText)
	overlayString(&body.DeviceClassPersonal, m.PersonalDeviceButtonName)
	overlayString(&body.DeviceClassEnterprise, m.InstitutionalOwnershipButtonName)
	overlayString(&body.DeviceClassPersonalDescription, m.PersonalDeviceManagementDescription)
	overlayString(&body.DeviceClassEnterpriseDescription, m.InstitutionalDeviceManagementDescription)
	overlayString(&body.DeviceClassButton, m.EnrollDeviceButtonName)

	overlayString(&body.PersonalEula, m.PersonalEula)
	overlayString(&body.EnterpriseEula, m.InstitutionalEula)
	overlayString(&body.EulaButton, m.EulaAcceptButtonText)

	overlayString(&body.SiteDescription, m.SiteSelectionText)

	overlayString(&body.CertificateText, m.CaCertificateInstallationText)
	overlayString(&body.CertificateProfileName, m.CaCertificateName)
	overlayString(&body.CertificateProfileDescription, m.CaCertificateDescription)
	overlayString(&body.CertificateButton, m.CaCertificateInstallButtonName)

	overlayString(&body.EnterpriseText, m.InstitutionalMdmInstallationText)
	overlayString(&body.EnterpriseProfileName, m.InstitutionalMdmProfileName)
	overlayString(&body.EnterpriseProfileDescription, m.InstitutionalMdmProfileDescription)
	overlayString(&body.EnterprisePending, m.InstitutionalMdmPendingText)
	overlayString(&body.EnterpriseButton, m.InstitutionalMdmInstallButtonName)

	overlayString(&body.UserEnrollmentText, m.UserEnrollmentMdmInstallationText)
	overlayString(&body.UserEnrollmentProfileName, m.UserEnrollmentMdmProfileName)
	overlayString(&body.UserEnrollmentProfileDescription, m.UserEnrollmentMdmProfileDescription)
	overlayString(&body.UserEnrollmentButton, m.UserEnrollmentMdmInstallButtonName)

	overlayString(&body.QuickAddText, m.QuickaddInstallationText)
	overlayString(&body.QuickAddName, m.QuickaddName)
	overlayString(&body.QuickAddPending, m.QuickaddProgressText)
	overlayString(&body.QuickAddButton, m.QuickaddInstallButtonName)

	overlayString(&body.CompleteMessage, m.EnrollmentCompleteText)
	overlayString(&body.FailedMessage, m.EnrollmentFailedText)
	overlayString(&body.TryAgainButton, m.TryAgainButtonName)
	overlayString(&body.CheckNowButton, m.ViewEnrollmentStatusButtonName)
	overlayString(&body.CheckEnrollmentMessage, m.ViewEnrollmentStatusText)
	overlayString(&body.LogoutButton, m.LogOutButtonName)

	return &body
}

// overlayString reassigns *dst to the plan value when it is known (not
// null/unknown), otherwise leaves *dst pointing at the seed's value.
func overlayString(dst **string, v types.String) {
	if v.IsNull() || v.IsUnknown() {
		return
	}
	s := v.ValueString()
	*dst = &s
}

// projectManagedMessagingLanguages returns the subset of the fresh server list
// corresponding to the declared codes. A declared code with no server match
// (should not happen after a successful reconcile) is dropped. The result has
// the same cardinality as the declared map, so the applied collection matches
// the plan.
func projectManagedMessagingLanguages(declared map[string]messagingLanguageModel, current []pro.EnrollmentProcessTextObject) []pro.EnrollmentProcessTextObject {
	byCode := make(map[string]pro.EnrollmentProcessTextObject, len(current))
	for _, c := range current {
		byCode[pointerString(c.LanguageCode)] = c
	}
	out := make([]pro.EnrollmentProcessTextObject, 0, len(declared))
	for code := range declared {
		if c, ok := byCode[code]; ok {
			out = append(out, c)
		}
	}
	return out
}

// messagingLanguagesToMap builds the messaging_languages map (keyed by language
// code) from a /v3 list response (or a managed subset of it).
func messagingLanguagesToMap(ctx context.Context, langs []pro.EnrollmentProcessTextObject) (types.Map, diag.Diagnostics) {
	elemType := types.ObjectType{AttrTypes: messagingLanguageAttrTypes}
	elems := make(map[string]attr.Value, len(langs))
	for i := range langs {
		obj, d := messagingLanguageObject(langs[i])
		if d.HasError() {
			return types.MapNull(elemType), d
		}
		elems[pointerString(langs[i].LanguageCode)] = obj
	}
	return types.MapValue(elemType, elems)
}

// messagingString maps a wire *string to a KNOWN, non-null Terraform string,
// preserving the empty string. Enrollment messaging fields are frequently empty
// ("") on the wire; the usual StringPointerValueOrNull helper collapses "" to
// null, which would make the Optional+Computed leaf's UseNonNullStateForUnknown
// plan modifier leave it unknown on every re-plan (it only copies NON-null prior
// state) — a perpetual "(known after apply)" diff. Keeping "" as a known empty
// string makes the value stable across plans.
func messagingString(s *string) types.String {
	if s == nil {
		return types.StringValue("")
	}
	return types.StringValue(*s)
}

// messagingLanguageObject converts an SDK text object into a Terraform object
// value (the map VALUE — language_code is the key, not an attribute here). The 4
// unmodelled personal* wire fields are not projected.
func messagingLanguageObject(l pro.EnrollmentProcessTextObject) (attr.Value, diag.Diagnostics) {
	return types.ObjectValue(messagingLanguageAttrTypes, map[string]attr.Value{
		"name":       messagingString(l.Name),
		"page_title": messagingString(l.Title),

		"login_page_text":   messagingString(l.LoginDescription),
		"username_text":     messagingString(l.Username),
		"password_text":     messagingString(l.Password),
		"login_button_text": messagingString(l.LoginButton),

		"device_ownership_page_text":                  messagingString(l.DeviceClassDescription),
		"personal_device_button_name":                 messagingString(l.DeviceClassPersonal),
		"institutional_ownership_button_name":         messagingString(l.DeviceClassEnterprise),
		"personal_device_management_description":      messagingString(l.DeviceClassPersonalDescription),
		"institutional_device_management_description": messagingString(l.DeviceClassEnterpriseDescription),
		"enroll_device_button_name":                   messagingString(l.DeviceClassButton),

		"personal_eula":           messagingString(l.PersonalEula),
		"institutional_eula":      messagingString(l.EnterpriseEula),
		"eula_accept_button_text": messagingString(l.EulaButton),

		"site_selection_text": messagingString(l.SiteDescription),

		"ca_certificate_installation_text":   messagingString(l.CertificateText),
		"ca_certificate_name":                messagingString(l.CertificateProfileName),
		"ca_certificate_description":         messagingString(l.CertificateProfileDescription),
		"ca_certificate_install_button_name": messagingString(l.CertificateButton),

		"institutional_mdm_installation_text":   messagingString(l.EnterpriseText),
		"institutional_mdm_profile_name":        messagingString(l.EnterpriseProfileName),
		"institutional_mdm_profile_description": messagingString(l.EnterpriseProfileDescription),
		"institutional_mdm_pending_text":        messagingString(l.EnterprisePending),
		"institutional_mdm_install_button_name": messagingString(l.EnterpriseButton),

		"user_enrollment_mdm_installation_text":   messagingString(l.UserEnrollmentText),
		"user_enrollment_mdm_profile_name":        messagingString(l.UserEnrollmentProfileName),
		"user_enrollment_mdm_profile_description": messagingString(l.UserEnrollmentProfileDescription),
		"user_enrollment_mdm_install_button_name": messagingString(l.UserEnrollmentButton),

		"quickadd_installation_text":   messagingString(l.QuickAddText),
		"quickadd_name":                messagingString(l.QuickAddName),
		"quickadd_progress_text":       messagingString(l.QuickAddPending),
		"quickadd_install_button_name": messagingString(l.QuickAddButton),

		"enrollment_complete_text":           messagingString(l.CompleteMessage),
		"enrollment_failed_text":             messagingString(l.FailedMessage),
		"try_again_button_name":              messagingString(l.TryAgainButton),
		"view_enrollment_status_button_name": messagingString(l.CheckNowButton),
		"view_enrollment_status_text":        messagingString(l.CheckEnrollmentMessage),
		"log_out_button_name":                messagingString(l.LogoutButton),
	})
}
