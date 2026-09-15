// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// englishLang returns a fully-populated English text object, including the four
// unmodelled personal* fields, to exercise seed-merge and preservation.
func englishLang() pro.EnrollmentProcessTextObject {
	return pro.EnrollmentProcessTextObject{
		LanguageCode:               new("en"),
		Name:                       new("English"),
		Title:                      new("Enroll Your Device"),
		LoginButton:                new("Log in"),
		DeviceClassPersonal:        new("Personally Owned"),
		EnterpriseButton:           new("Continue"),
		QuickAddButton:             new("Download"),
		CompleteMessage:            new("Done"),
		LogoutButton:               new("Log Out"),
		PersonalText:               new("legacy personal text"),   // unmodelled
		PersonalButton:             new("legacy personal button"), // unmodelled
		PersonalProfileName:        new("legacy personal name"),   // unmodelled
		PersonalProfileDescription: new("legacy personal desc"),   // unmodelled
	}
}

// langModel builds a messagingLanguageModel declaring only name + page_title;
// all other fields are null (unset). language_code is the map key, not a field.
func langModel(name, pageTitle string) messagingLanguageModel {
	return messagingLanguageModel{
		Name:      types.StringValue(name),
		PageTitle: types.StringValue(pageTitle),
	}
}

// TestBuildMessagingLanguageInput_SeedMergeAndPreserve proves declared fields
// overlay, unset fields are preserved from the seed, the 4 unmodelled personal*
// fields ride along unchanged, and code/name are set explicitly.
func TestBuildMessagingLanguageInput_SeedMergeAndPreserve(t *testing.T) {
	seed := englishLang()
	m := langModel("French", "Inscrivez votre appareil")

	body := buildMessagingLanguageInput(seed, m, "fr", "French")

	if pointerString(body.LanguageCode) != "fr" || pointerString(body.Name) != "French" {
		t.Fatalf("code/name not set: %q/%q", pointerString(body.LanguageCode), pointerString(body.Name))
	}
	if pointerString(body.Title) != "Inscrivez votre appareil" {
		t.Errorf("declared page_title not overlaid, got %q", pointerString(body.Title))
	}
	// Unset field preserved from seed.
	if pointerString(body.LoginButton) != "Log in" {
		t.Errorf("unset login_button_text not preserved from seed, got %q", pointerString(body.LoginButton))
	}
	// Unmodelled personal* fields ride along unchanged.
	if pointerString(body.PersonalText) != "legacy personal text" {
		t.Errorf("unmodelled personalText not preserved, got %q", pointerString(body.PersonalText))
	}
	// Seed must not be mutated.
	if pointerString(seed.Title) != "Enroll Your Device" {
		t.Errorf("seed was mutated, Title now %q", pointerString(seed.Title))
	}
}

// TestPlanMessagingLanguage_NeverDeletesEnglish proves English is never
// scheduled for delete even when the planned map omits every language.
func TestPlanMessagingLanguage_NeverDeletesEnglish(t *testing.T) {
	current := []pro.EnrollmentProcessTextObject{
		englishLang(),
		{LanguageCode: new("fr"), Name: new("French"), Title: new("Bonjour")},
	}
	// Planned map is empty: user manages the collection to "none".
	ops := planMessagingLanguageReconcile(nil, current, englishLang(), nil)

	var deletes []string
	for _, op := range ops {
		if op.Action == messagingLanguageDelete {
			deletes = append(deletes, op.Code)
			if op.Code == defaultLanguageCode {
				t.Fatal("English must never be deleted")
			}
		}
	}
	if len(deletes) != 1 || deletes[0] != "fr" {
		t.Fatalf("expected only fr deleted, got %v", deletes)
	}
}

// TestPlanMessagingLanguage_CreateUpdateDelete covers all three actions in one
// reconcile.
func TestPlanMessagingLanguage_CreateUpdateDelete(t *testing.T) {
	current := []pro.EnrollmentProcessTextObject{
		englishLang(),
		{LanguageCode: new("fr"), Name: new("French"), Title: new("Bonjour")},
		{LanguageCode: new("es"), Name: new("Spanish"), Title: new("Hola")},
	}
	planned := map[string]messagingLanguageModel{
		"fr": langModel("French", "Salut"), // update (title changed)
		"de": langModel("German", "Hallo"), // create
		// en omitted → protected; es omitted → delete.
	}
	names := map[string]string{"de": "German"}

	ops := planMessagingLanguageReconcile(planned, current, englishLang(), names)

	var upserts, deletes []string
	for _, op := range ops {
		switch op.Action {
		case messagingLanguageUpsert:
			upserts = append(upserts, op.Code)
		case messagingLanguageDelete:
			deletes = append(deletes, op.Code)
		}
	}
	if len(upserts) != 2 {
		t.Errorf("expected 2 upserts (fr,de), got %v", upserts)
	}
	if len(deletes) != 1 || deletes[0] != "es" {
		t.Errorf("expected only es deleted, got %v", deletes)
	}
	// The created "de" body must be seeded from English (LoginButton) + named.
	for _, op := range ops {
		if op.Code == "de" {
			if pointerString(op.Body.Name) != "German" {
				t.Errorf("de name not resolved, got %q", pointerString(op.Body.Name))
			}
			if pointerString(op.Body.LoginButton) != "Log in" {
				t.Errorf("de not seeded from English, login_button_text=%q", pointerString(op.Body.LoginButton))
			}
		}
	}
}

// TestPlanMessagingLanguage_NoChangeNoOps proves a declared language identical
// to the server yields no operations.
func TestPlanMessagingLanguage_NoChangeNoOps(t *testing.T) {
	current := []pro.EnrollmentProcessTextObject{
		englishLang(),
		{LanguageCode: new("fr"), Name: new("French"), Title: new("Bonjour")},
	}
	planned := map[string]messagingLanguageModel{
		"fr": langModel("French", "Bonjour"), // identical to current
	}
	if ops := planMessagingLanguageReconcile(planned, current, englishLang(), nil); len(ops) != 0 {
		t.Errorf("expected no ops for unchanged plan, got %+v", ops)
	}
}

// TestProjectManagedMessagingLanguages_Cardinality proves the readback
// projection returns only the declared subset, never appending undeclared
// languages (so the applied map keys equal the planned keys).
func TestProjectManagedMessagingLanguages_Cardinality(t *testing.T) {
	current := []pro.EnrollmentProcessTextObject{
		englishLang(), // undeclared
		{LanguageCode: new("fr"), Name: new("French"), Title: new("Bonjour")},
	}
	declared := map[string]messagingLanguageModel{"fr": langModel("French", "Bonjour")}

	got := projectManagedMessagingLanguages(declared, current)
	if len(got) != 1 || pointerString(got[0].LanguageCode) != "fr" {
		t.Fatalf("expected only fr, got %+v", got)
	}
}

// TestMessagingLanguagesToMap_BuildsMap proves the map assigner keys by language
// code and maps the modelled fields (the personal* fields have no attribute).
func TestMessagingLanguagesToMap_BuildsMap(t *testing.T) {
	langs := []pro.EnrollmentProcessTextObject{englishLang()}
	m, d := messagingLanguagesToMap(context.Background(), langs)
	if d.HasError() {
		t.Fatalf("unexpected diagnostics: %v", d)
	}
	if _, ok := m.Elements()["en"]; !ok {
		t.Fatalf("expected map keyed by \"en\", got keys %v", m.Elements())
	}
}
