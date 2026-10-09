// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ssodomainaction

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
)

func verifySchema(t *testing.T) actionschema.Schema {
	t.Helper()
	a := NewVerifySSODomainAction()
	var resp action.SchemaResponse
	a.(*VerifySSODomainAction).Schema(context.Background(), action.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// description returns the action description without the generated permissions
// table, which carries a documentation URL that every vocabulary check would trip
// over.
func description(t *testing.T) string {
	t.Helper()
	return strings.TrimSuffix(verifySchema(t).MarkdownDescription, verifySSODomainPrivileges)
}

func TestVerifySSODomainAction_Metadata(t *testing.T) {
	a := NewVerifySSODomainAction()
	var resp action.MetadataResponse
	a.(*VerifySSODomainAction).Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	want := "jamfplatform_account_sso_domain_verify"
	if resp.TypeName != want {
		t.Errorf("type name = %q, want %q", resp.TypeName, want)
	}
}

// TestVerifySSODomainAction_Attributes pins the identifier shape: both forms are
// optional individually because exactly one is required, and that is enforced by a
// ConfigValidator rather than per-attribute rules so that naming neither also fails
// at plan time.
func TestVerifySSODomainAction_Attributes(t *testing.T) {
	s := verifySchema(t)

	for _, name := range []string{"domain", "domain_id"} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("missing attribute %s", name)
			continue
		}
		if attr.IsRequired() {
			t.Errorf("%s must be optional — exactly one of the two identifies the domain", name)
		}
		if !attr.IsOptional() {
			t.Errorf("%s must be optional", name)
		}
	}

	if len(s.Attributes) != 3 {
		names := make([]string, 0, len(s.Attributes))
		for name := range s.Attributes {
			names = append(names, name)
		}
		sort.Strings(names)
		t.Errorf("expected exactly domain, domain_id and timeouts, got %v", names)
	}
}

// TestVerifySSODomainAction_AttributesAreNeitherSensitiveNorWriteOnly pins a
// framework limit that is easy to forget: an action attribute can be neither, so
// nothing secret may be accepted here. Both identifiers are public names, which is
// what makes this action safe to shape this way.
func TestVerifySSODomainAction_AttributesAreNeitherSensitiveNorWriteOnly(t *testing.T) {
	s := verifySchema(t)

	for name, attr := range s.Attributes {
		if attr.IsSensitive() {
			t.Errorf("%s is Sensitive, which an action attribute cannot be", name)
		}
		if attr.IsWriteOnly() {
			t.Errorf("%s is WriteOnly, which an action attribute cannot be", name)
		}
	}
}

// TestVerifySSODomainAction_ExactlyOneIdentifier pins that the exactly-one rule is
// declared. Without it, an invocation naming neither identifier reaches Invoke with
// nothing to act on, and one naming both silently ignores whichever loses.
func TestVerifySSODomainAction_ExactlyOneIdentifier(t *testing.T) {
	a := NewVerifySSODomainAction()

	validators := a.(*VerifySSODomainAction).ConfigValidators(context.Background())
	if len(validators) != 1 {
		t.Fatalf("expected one config validator, got %d", len(validators))
	}

	desc := validators[0].MarkdownDescription(context.Background())
	for _, fragment := range []string{"domain", "domain_id"} {
		if !strings.Contains(desc, fragment) {
			t.Errorf("the config validator does not cover %q: %s", fragment, desc)
		}
	}
}

// TestVerifySSODomainAction_DescriptionSaysFailureFails pins the honesty the whole
// action rests on: a verification that proves nothing is reported by Jamf the same
// way as one that succeeds, so the description has to say that this action turns
// the unproven case into a failure rather than a silent pass.
func TestVerifySSODomainAction_DescriptionSaysFailureFails(t *testing.T) {
	desc := description(t)

	for _, fragment := range []string{"does not succeed fails the run", "still not proven", "verification status"} {
		if !strings.Contains(desc, fragment) {
			t.Errorf("description does not say what an unsuccessful verification does: missing %q\n%s", fragment, desc)
		}
	}
}

// TestVerifySSODomainAction_DescriptionSaysRateLimited pins that the five-minute
// limit and its cause are documented. A practitioner whose first verification is
// refused has to be able to tell "wait" from "misconfigured", and the cause is not
// guessable: claiming the domain is what starts the clock.
func TestVerifySSODomainAction_DescriptionSaysRateLimited(t *testing.T) {
	desc := description(t)

	for _, fragment := range []string{"one verification every five minutes", "claiming it counts as a change",
		"rather than waiting it"} {
		if !strings.Contains(desc, fragment) {
			t.Errorf("description does not document the five-minute limit: missing %q\n%s", fragment, desc)
		}
	}
}

// TestVerifySSODomainAction_DescriptionWarnsAgainstTriggeringFromTheClaim pins the
// one wiring that can never work, and the one that can. The five-minute limit is
// counted from the claim, so an action_trigger on the claiming resource is refused
// on every fresh claim rather than occasionally — which makes the obvious
// arrangement, and the one every sibling action example in this provider uses, the
// guaranteed-failing one here. Both the action description and the identifier a
// practitioner reaches for first have to say so, and both have to show the
// standalone invocation that does work.
func TestVerifySSODomainAction_DescriptionWarnsAgainstTriggeringFromTheClaim(t *testing.T) {
	s := verifySchema(t)

	name, ok := s.Attributes["domain"]
	if !ok {
		t.Fatal("missing attribute domain")
	}

	for _, desc := range []string{description(t), name.GetMarkdownDescription()} {
		for _, fragment := range []string{"Do not trigger this action from the", "-invoke="} {
			if !strings.Contains(desc, fragment) {
				t.Errorf("description does not steer away from lifecycle-triggering: missing %q\n%s", fragment, desc)
			}
		}
	}
}

// TestVerifySSODomainAction_DescriptionSaysItIsNotIdempotent pins the third wire
// fact, and the one a practitioner has no way at all of discovering: every
// invocation moves two of the domain's timestamps, so repeatedly triggering it is
// not free.
func TestVerifySSODomainAction_DescriptionSaysItIsNotIdempotent(t *testing.T) {
	desc := description(t)

	for _, fragment := range []string{"never free", "14 days", "last_modified_at", "verification_expires_at"} {
		if !strings.Contains(desc, fragment) {
			t.Errorf("description does not document the mutation every invocation causes: missing %q\n%s", fragment, desc)
		}
	}
}

// TestVerifySSODomainAction_IdentifierDescriptionsGuideTheChoice pins that each
// identifier says when to use it. The name is what a practitioner holds; the
// identifier is not shown anywhere in Jamf Account, so an attribute offering it
// without saying where it comes from is a dead end.
func TestVerifySSODomainAction_IdentifierDescriptionsGuideTheChoice(t *testing.T) {
	s := verifySchema(t)

	name, ok := s.Attributes["domain"]
	if !ok {
		t.Fatal("missing attribute domain")
	}
	for _, fragment := range []string{"Case is ignored", "jamfplatform_account_sso_domain", "never both"} {
		if !strings.Contains(name.GetMarkdownDescription(), fragment) {
			t.Errorf("domain description does not mention %q:\n%s", fragment, name.GetMarkdownDescription())
		}
	}

	id, ok := s.Attributes["domain_id"]
	if !ok {
		t.Fatal("missing attribute domain_id")
	}
	for _, fragment := range []string{"never shows this identifier", "jamfplatform_account_sso_domain", "never both"} {
		if !strings.Contains(id.GetMarkdownDescription(), fragment) {
			t.Errorf("domain_id description does not mention %q:\n%s", fragment, id.GetMarkdownDescription())
		}
	}
}

// TestVerifySSODomainAction_DescriptionsAreUIAligned pins STYLE_GUIDE §User-facing
// descriptions are UI-aligned, not wire-aligned. The wire facts this action is
// shaped around are real and load-bearing, which makes it especially tempting to
// state them in wire terms — they belong in the Go doc comments instead.
func TestVerifySSODomainAction_DescriptionsAreUIAligned(t *testing.T) {
	s := verifySchema(t)

	descriptions := []string{description(t)}
	for _, attr := range s.Attributes {
		descriptions = append(descriptions, attr.GetMarkdownDescription())
	}

	for _, desc := range descriptions {
		lower := strings.ToLower(desc)
		for _, banned := range []string{"endpoint", "/v1/", " sdk", " api", "http", "payload", "200", "400",
			"domainstatus", "lastmodifieddate"} {
			if strings.Contains(lower, banned) {
				t.Errorf("description contains wire vocabulary %q:\n%s", banned, desc)
			}
		}
	}
}
