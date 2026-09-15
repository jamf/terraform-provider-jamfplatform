// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tripwire for the day-offset encoding the no-execute window needs. It holds
// the unit-level encoder against the live endpoint, reaching it through the
// EncodeNoExecuteTimeForTest bridge in export_test.go — testhelpers imports the
// provider, which imports this package, so an internal-package acceptance test
// would be an import cycle. See no_execute.go and Jamf PI-1661.

package policy_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/policy"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccPolicyResource_NoExecuteWindowEncoding goes round the provider and
// writes the window through the SDK directly: once with a plain time, which
// Jamf Pro currently discards, and once with the day-offset form, which it
// currently stores.
//
// EITHER ASSERTION FAILING IS NEWS:
//
//   - the plain write starting to persist means the defect is fixed. Have
//     noExecuteTimePointer send the value verbatim, delete encodeNoExecuteTime
//     and its unit tests, delete this file, and close PI-1661.
//   - the encoded write stopping means the arithmetic changed. The provider is
//     now silently failing to set anybody's window; re-probe before shipping.
//
// The write goes through the SDK rather than raw HTTP for the reason in
// feedback_probe_through_the_sdk_not_curl: it is the path the provider takes,
// so a behaviour change the SDK's marshalling cannot express is not one this
// provider could adopt.
func TestAccPolicyResource_NoExecuteWindowEncoding(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ctx := context.Background()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	name := "tf-acc-policy-noexec-canary-" + testhelpers.RunSuffix()
	disabled := false

	id, _, err := c.ApplyPolicy(ctx, &proclassic.PolicyPost{
		General: &proclassic.PolicyPostGeneral{Name: &name, Enabled: &disabled},
	})
	if err != nil {
		t.Fatalf("create canary policy: %s", err)
	}
	t.Cleanup(func() {
		if err := c.DeletePolicyByID(ctx, id); err != nil {
			t.Logf("cleanup: delete policy %s: %s", id, err)
		}
	})

	// write sends one no-execute start through the SDK and returns what the
	// server stored. no_execute_on travels with it as a control: it persists
	// unconditionally, so an empty one means the request never landed and the
	// result says nothing about the encoding.
	write := func(t *testing.T, value string) string {
		t.Helper()
		if err := c.UpdatePolicyByID(ctx, id, &proclassic.PolicyPost{
			General: &proclassic.PolicyPostGeneral{
				DateTimeLimitations: &proclassic.PolicyGeneralDateTimeLimitations{
					NoExecuteOn:    &proclassic.PolicyGeneralDateTimeLimitationsNoExecuteOn{Day: &[]string{"Sun", "Sat"}},
					NoExecuteStart: &value,
				},
			},
		}); err != nil {
			t.Fatalf("write %q: %s", value, err)
		}
		got, err := c.GetPolicyByID(ctx, id)
		if err != nil {
			t.Fatalf("read back after writing %q: %s", value, err)
		}
		dtl := got.General.DateTimeLimitations
		if dtl == nil {
			t.Fatalf("policy %s came back with no date_time_limitations at all", id)
		}
		if dtl.NoExecuteOn == nil || dtl.NoExecuteOn.Day == nil || len(*dtl.NoExecuteOn.Day) == 0 {
			t.Fatalf("no_execute_on did not persist either — the write did not land, so this proves nothing about %q", value)
		}
		if dtl.NoExecuteStart == nil {
			return ""
		}
		return strings.TrimSpace(*dtl.NoExecuteStart)
	}

	t.Run("a plain time is still discarded", func(t *testing.T) {
		if got := write(t, "5:00 PM"); got != "" {
			t.Fatalf(
				"Jamf Pro now stores a plain no-execute time (sent \"5:00 PM\", stored %q).\n\n"+
					"If it stored it faithfully this is the PI-1661 fix landing, and the encoding must go:\n"+
					"  1. have noExecuteTimePointer in no_execute.go send the value verbatim\n"+
					"  2. delete encodeNoExecuteTime, its unit tests and the export_test.go bridge\n"+
					"  3. delete this file\n"+
					"  4. close Jamf PI-1661",
				got,
			)
		}
	})

	t.Run("the encoder agrees with the wire", func(t *testing.T) {
		// Ties the unit-level encoder to the live endpoint, so a change to
		// either side that the other does not follow shows up here rather than
		// as a silent no-op in production. Midnight and noon are included
		// because both sit on arithmetic boundaries in encodeNoExecuteTime.
		for _, want := range []string{"12:00 AM", "12:01 AM", "1:00 AM", "11:59 AM", "12:00 PM", "12:30 PM", "5:15 PM", "11:59 PM"} {
			sent := policy.EncodeNoExecuteTimeForTest(want)
			if got := write(t, sent); got != want {
				t.Errorf(
					"encodeNoExecuteTime(%q) produced %q, which Jamf Pro stored as %q.\n"+
						"The provider is silently failing to set this window. Re-probe the endpoint's "+
						"arithmetic — see no_execute.go for the method.",
					want, sent, got,
				)
			}
		}
	})
}
