// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
)

// A freshly-created LDAP server fixture is not queryable the instant the POST
// returns — Jamf needs a moment to establish the bind/connection before
// /v1/ldap/groups surfaces its groups. ResolveDSGroupWireValue rides out that gap.
const (
	dsGroupResolveTimeout  = 120 * time.Second
	dsGroupResolveInterval = 5 * time.Second
)

// NewProClassicClient builds a ProClassic SDK client from the acceptance client.
func NewProClassicClient(t *testing.T) *proclassic.Client {
	t.Helper()
	return proclassic.New(NewAcceptanceClient(t))
}

// ResolveComputerIDByName returns the Jamf Pro computer id for a computer name.
//
// It reads the Pro computer inventory rather than the classic by-name lookup:
// Jamf deprecated that endpoint (2025-02-11) along with the rest of the classic
// computer-listing surface, so there is no non-deprecated classic equivalent to
// fall back on. Fixtures need this because the classic create call returns the
// new id on the wire but the SDK signature discards it.
//
// Wire-probed 2026-08-03: a computer created through the classic endpoint is
// visible in the Pro inventory immediately and under the same id, so this is
// safe to call directly after a create with no settling delay.
func ResolveComputerIDByName(ctx context.Context, t *testing.T, name string) string {
	t.Helper()

	matches, err := pro.New(NewAcceptanceClient(t)).ListComputersInventoryV4(
		ctx, []string{pro.ComputerSectionV4General}, nil, fmt.Sprintf("general.name==%q", name),
	)
	if err != nil {
		t.Fatalf("resolving computer %q: %v", name, err)
	}
	if len(matches) == 0 {
		t.Fatalf("no computer matched name %q", name)
	}
	return matches[0].ID
}

// RequireSingletonStillExists returns a CheckDestroy that asserts a Pro singleton
// record still exists on the tenant after Terraform destroy — the convention for
// update-only singleton resources whose Delete is a no-op. label names the record
// in failure messages; get performs the resource-specific SDK read. A nil record
// (including a typed-nil pointer) or a read error fails the check.
func RequireSingletonStillExists(t *testing.T, label string, get func(context.Context) (any, error)) resource.TestCheckFunc {
	t.Helper()
	return func(_ *terraform.State) error {
		got, err := get(context.Background())
		if err != nil {
			return fmt.Errorf("expected %s record to persist on tenant after destroy, got error: %w", label, err)
		}
		if got == nil {
			return fmt.Errorf("expected non-nil %s record post-destroy", label)
		}
		if rv := reflect.ValueOf(got); rv.Kind() == reflect.Pointer && rv.IsNil() {
			return fmt.Errorf("expected non-nil %s record post-destroy", label)
		}
		return nil
	}
}

// ResolveDSGroupWireValue resolves a directory-service group by name to its encoded
// wire value (uuid + ldap server id). Fails the test unless the name resolves to
// exactly one group with a uuid mapping. The value is computed via the same
// criteria.EncodeDSGroupValue the provider uses, so the acc test's name<->base64
// swap step is byte-aligned with the provider; the live apply itself is the
// independent check that the encoding is server-acceptable.
func ResolveDSGroupWireValue(t *testing.T, name string) string {
	t.Helper()
	groups := WaitForLdapGroupResolvable(t, name)
	if len(groups) != 1 {
		t.Fatalf("group %q must resolve to exactly one directory group (got %d) — pick an unambiguous name", name, len(groups))
	}
	if groups[0].UUID == "" {
		t.Fatalf("group %q resolved with no uuid mapping", name)
	}
	return criteria.EncodeDSGroupValue(groups[0].UUID, strconv.Itoa(groups[0].LdapServerID))
}

// WaitForLdapGroupResolvable polls /v1/ldap/groups until name resolves to at least
// one exact match, returning the matches. It rides out the connection-establishment
// delay on a freshly-created LDAP fixture server (the bind is not ready the instant
// the create POST returns). Call it after EnsureLdapServerFixture in any test that
// then resolves the group — directly (ResolveDSGroupWireValue) or via the provider's
// plan-time scope preflight. Fatals on a persistent empty result or search error
// past the timeout (a real config failure, not a transient delay).
func WaitForLdapGroupResolvable(t *testing.T, name string) []ldapgroups.Group {
	t.Helper()
	resolver := pro.New(NewAcceptanceClient(t))
	deadline := time.Now().Add(dsGroupResolveTimeout)
	for {
		groups, err := ldapgroups.ResolveByName(context.Background(), resolver, name)
		if err == nil && len(groups) >= 1 {
			return groups
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("resolving %q within %s: %v", name, dsGroupResolveTimeout, err)
			}
			t.Fatalf("group %q did not resolve within %s (search returned no exact match — check the group exists in the directory and the LDAP fixture surfaces it)", name, dsGroupResolveTimeout)
		}
		time.Sleep(dsGroupResolveInterval)
	}
}
