// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// scriptedReader returns a gatewayReader that answers with the given states in
// order, repeating the last one once exhausted, and a pointer to the read count.
//
// The reads are what the assertions are really about: the wait's contract is as
// much about how many times it goes to the API as about what it concludes.
func scriptedReader(states ...string) (gatewayReader, *int) {
	reads := 0
	return func(_ context.Context, _ string) (*securitycloud.Gateway, error) {
		state := states[min(reads, len(states)-1)]
		reads++
		return &securitycloud.Gateway{Status: &securitycloud.GatewayStatus{State: state}}, nil
	}, &reads
}

// TestWaitForGatewayState_SatisfiedOnlyByUp pins which reported states end the wait.
//
// UP ends it. PENDING does not, which is the whole point. DOWN does not either, and
// specifically does not end it as a failure: the 2026-08-31 internet-form probe never
// produced DOWN, so whether it can appear transiently mid-provisioning on that form is
// unknown, and treating it as terminal on that ignorance would fail an apply that was
// about to succeed. The same day's IPsec probe did settle at DOWN, which is why that
// form is excluded from the wait entirely rather than being made to fail fast here.
func TestWaitForGatewayState_SatisfiedOnlyByUp(t *testing.T) {
	cases := []struct {
		name      string
		states    []string
		wantReads int
	}{
		{
			name:      "already up ends the wait on the first read",
			states:    []string{securitycloud.GatewayStatusStateUp},
			wantReads: 1,
		},
		{
			name: "pending is not terminal",
			states: []string{
				securitycloud.GatewayStatusStatePending,
				securitycloud.GatewayStatusStatePending,
				securitycloud.GatewayStatusStateUp,
			},
			wantReads: 3,
		},
		{
			name: "down is not terminal either",
			states: []string{
				securitycloud.GatewayStatusStateDown,
				securitycloud.GatewayStatusStateDown,
				securitycloud.GatewayStatusStateUp,
			},
			wantReads: 3,
		},
		{
			name: "a disabled report is not terminal",
			states: []string{
				securitycloud.GatewayStatusStateDisabled,
				securitycloud.GatewayStatusStateUp,
			},
			wantReads: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			read, reads := scriptedReader(tc.states...)
			observed, lastState, reachedUp := waitForGatewayState(context.Background(), read, "a1b2", []string{securitycloud.GatewayStatusStateUp}, time.Millisecond, "", 0)

			if !reachedUp {
				t.Fatalf("the wait was not satisfied; last state %q after %d reads", lastState, *reads)
			}
			if lastState != securitycloud.GatewayStatusStateUp {
				t.Errorf("last state = %q, want %q", lastState, securitycloud.GatewayStatusStateUp)
			}
			if *reads != tc.wantReads {
				t.Errorf("reads = %d, want %d", *reads, tc.wantReads)
			}
			if observed == nil {
				t.Error("the wait must hand back the representation it read, or the caller pays for a further read")
			}
		})
	}
}

// TestWaitForGatewayState_AlreadyUpDoesNotSleep pins the property that makes the update
// path affordable.
//
// An update that does not re-provision the gateway — a name or contact change — finds
// it already UP. Waiting has to cost nothing in that case, because it is what lets
// Update wait unconditionally instead of trying to work out from the plan whether the
// change re-provisions anything. The interval here is far longer than the assertion
// window, so a single sleep would fail the test.
func TestWaitForGatewayState_AlreadyUpDoesNotSleep(t *testing.T) {
	read, reads := scriptedReader(securitycloud.GatewayStatusStateUp)

	started := time.Now()
	_, _, reachedUp := waitForGatewayState(context.Background(), read, "a1b2", []string{securitycloud.GatewayStatusStateUp}, 30*time.Second, "", 0)
	elapsed := time.Since(started)

	if !reachedUp {
		t.Fatal("an already-operational gateway must satisfy the wait")
	}
	if *reads != 1 {
		t.Errorf("reads = %d, want 1", *reads)
	}
	if elapsed > time.Second {
		t.Errorf("the wait slept for %s on an already-operational gateway; it must return immediately", elapsed)
	}
}

// TestWaitForGatewayState_BudgetExpiryReportsTheStateReached pins what the wait hands
// back when the caller's context budget runs out: not satisfied, plus the last state
// it saw, which is what the warning is worded from.
func TestWaitForGatewayState_BudgetExpiryReportsTheStateReached(t *testing.T) {
	cases := []struct {
		name  string
		state string
	}{
		{name: "stuck provisioning", state: securitycloud.GatewayStatusStatePending},
		{name: "reported down", state: securitycloud.GatewayStatusStateDown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			read, reads := scriptedReader(tc.state)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()

			observed, lastState, reachedUp := waitForGatewayState(ctx, read, "a1b2", []string{securitycloud.GatewayStatusStateUp}, time.Millisecond, "", 0)

			if reachedUp {
				t.Fatal("a gateway that never reports itself operational must not satisfy the wait")
			}
			if lastState != tc.state {
				t.Errorf("last state = %q, want %q", lastState, tc.state)
			}
			if *reads < 2 {
				t.Errorf("reads = %d, want the wait to have kept polling", *reads)
			}
			if observed == nil {
				t.Error("the last successful read must survive an exhausted budget, or the caller must read again on a dead context")
			}
		})
	}
}

// TestWaitForGatewayState_ReadFailureEndsTheWait pins that a read failure stops the wait
// instead of being swallowed, matching waitForBenchmarkSync in
// internal/resources/cbengine/benchmark.
//
// Swallowing it would turn a broken credential into a full-budget spin. Ending the
// wait costs nothing, because the caller reads the gateway again afterwards and that
// read produces the real error diagnostic.
func TestWaitForGatewayState_ReadFailureEndsTheWait(t *testing.T) {
	reads := 0
	read := func(_ context.Context, _ string) (*securitycloud.Gateway, error) {
		reads++
		return nil, errors.New("the read failed")
	}

	observed, lastState, reachedUp := waitForGatewayState(context.Background(), read, "a1b2", []string{securitycloud.GatewayStatusStateUp}, 30*time.Second, "", 0)

	if reachedUp {
		t.Fatal("a read failure must not satisfy the wait")
	}
	if reads != 1 {
		t.Errorf("reads = %d, want 1 — the wait must not retry a failing read", reads)
	}
	if lastState != "" {
		t.Errorf("last state = %q, want empty — nothing was ever read", lastState)
	}
	if observed != nil {
		t.Error("nothing was read, so the wait must hand back nothing")
	}
}

// TestWaitForGatewayState_DisabledTargetIgnoresUp pins that the disabled wait really
// waits for DISABLED, rather than for "anything that is not PENDING".
//
// Disabling a gateway was observed to report PENDING for a few seconds before
// settling to DISABLED (admin UI and API, 2026-08-31). That transient is the whole
// reason this wait exists: without it an apply records PENDING on a gateway the
// operator just disabled. A wait satisfied by UP, or by the first non-PENDING value,
// would record the wrong status just as skipping the wait did.
func TestWaitForGatewayState_DisabledTargetIgnoresUp(t *testing.T) {
	read, reads := scriptedReader(
		securitycloud.GatewayStatusStateUp,
		securitycloud.GatewayStatusStatePending,
		securitycloud.GatewayStatusStateDisabled,
	)

	observed, lastState, reached := waitForGatewayState(context.Background(), read, "a1b2", []string{securitycloud.GatewayStatusStateDisabled}, time.Millisecond, "", 0)
	if !reached {
		t.Fatalf("the wait must be satisfied by DISABLED, got lastState=%q", lastState)
	}
	if lastState != securitycloud.GatewayStatusStateDisabled {
		t.Errorf("lastState = %q, want DISABLED", lastState)
	}
	if observed == nil || observed.Status == nil || observed.Status.State != securitycloud.GatewayStatusStateDisabled {
		t.Errorf("the wait must hand back the disabled read, got %+v", observed)
	}
	if *reads != 3 {
		t.Errorf("reads = %d, want 3 — UP and PENDING must both have been rejected", *reads)
	}
}

// TestAppendGatewayWaitWarning_DisabledTargetHasItsOwnWording pins that an exhausted
// disabled wait does not borrow the coming-up wording, which would tell an operator
// their gateway is not carrying traffic as though that were a fault rather than the
// thing they just asked for.
func TestAppendGatewayWaitWarning_DisabledTargetHasItsOwnWording(t *testing.T) {
	var diags diag.Diagnostics
	appendGatewayWaitWarning(&diags, gatewayWaitUpdate, []string{securitycloud.GatewayStatusStateDisabled}, securitycloud.GatewayStatusStatePending)

	if diags.HasError() {
		t.Fatalf("an exhausted wait must never error: %v", diags)
	}
	if diags.WarningsCount() != 1 {
		t.Fatalf("want exactly one warning, got %d: %v", diags.WarningsCount(), diags)
	}
	d := diags.Warnings()[0]
	if !strings.Contains(d.Summary(), "disabled") {
		t.Errorf("the summary must say the gateway is not yet reported disabled, got %q", d.Summary())
	}
	for _, unwanted := range []string{"carrying traffic", "still provisioning"} {
		if strings.Contains(d.Detail(), unwanted) {
			t.Errorf("the disabled wording must not borrow the coming-up wording (%q): %s", unwanted, d.Detail())
		}
	}
	if !strings.Contains(d.Detail(), "timeouts") {
		t.Errorf("the warning must name the timeouts block: %s", d.Detail())
	}
}

// TestWaitForGatewayState_DwellOnDown pins the dwell, which is the whole reason an
// enabled IPsec gateway can be waited on at all.
//
// A DOWN that recovers inside the dwell must not end the wait: whether a gateway can
// report DOWN briefly before reaching UP is unmeasured, and the dwell exists so the
// answer does not matter. A DOWN that persists past it must end the wait, or an
// enabled IPsec gateway whose tunnel is not up would hang the apply for the whole
// ten-minute budget.
func TestWaitForGatewayState_DwellOnDown(t *testing.T) {
	t.Run("a DOWN that recovers inside the dwell still reaches UP", func(t *testing.T) {
		read, reads := scriptedReader(
			securitycloud.GatewayStatusStateDown,
			securitycloud.GatewayStatusStateDown,
			securitycloud.GatewayStatusStateUp,
		)

		_, lastState, reached := waitForGatewayState(context.Background(), read, "a1b2",
			[]string{securitycloud.GatewayStatusStateUp}, time.Millisecond,
			securitycloud.GatewayStatusStateDown, time.Hour)

		if !reached {
			t.Fatalf("a recovering gateway must reach UP, got lastState=%q after %d reads", lastState, *reads)
		}
		if lastState != securitycloud.GatewayStatusStateUp {
			t.Errorf("lastState = %q, want UP", lastState)
		}
	})

	t.Run("a DOWN that persists past the dwell ends the wait unreached", func(t *testing.T) {
		read, reads := scriptedReader(securitycloud.GatewayStatusStateDown)

		observed, lastState, reached := waitForGatewayState(context.Background(), read, "a1b2",
			[]string{securitycloud.GatewayStatusStateUp}, time.Millisecond,
			securitycloud.GatewayStatusStateDown, 10*time.Millisecond)

		if reached {
			t.Fatal("a gateway that stays DOWN must not be reported as having reached UP")
		}
		if lastState != securitycloud.GatewayStatusStateDown {
			t.Errorf("lastState = %q, want DOWN so the warning can name it", lastState)
		}
		if observed == nil {
			t.Error("the last read must survive, so the apply records what it saw")
		}
		if *reads < 2 {
			t.Errorf("reads = %d: the dwell must span at least two reads, or it is not a dwell", *reads)
		}
	})

	t.Run("no abandon state means DOWN never ends the wait", func(t *testing.T) {
		read, _ := scriptedReader(
			securitycloud.GatewayStatusStateDown,
			securitycloud.GatewayStatusStateDown,
			securitycloud.GatewayStatusStateDisabled,
		)

		_, lastState, reached := waitForGatewayState(context.Background(), read, "a1b2",
			[]string{securitycloud.GatewayStatusStateDisabled}, time.Millisecond, "", 0)

		if !reached || lastState != securitycloud.GatewayStatusStateDisabled {
			t.Errorf("a disable must wait through DOWN: reached=%v lastState=%q", reached, lastState)
		}
	})
}

// TestWaitForGatewayState_IPsecCreateSettlesOnEither pins the two-answer wait an
// enabled IPsec create runs, and that neither answer is favoured.
//
// DOWN is the settled status of a gateway whose tunnel is not built, which on a create
// is every gateway: its far end is configured from values the create returns. UP is
// still accepted because everything the far end needs is caller-supplied here, so an
// operator who pre-staged their concentrator can go straight from PENDING to UP. A
// wait for either is reachable in both cases; a wait for one alone would hang in the
// other.
func TestWaitForGatewayState_IPsecCreateSettlesOnEither(t *testing.T) {
	want := []string{securitycloud.GatewayStatusStateUp, securitycloud.GatewayStatusStateDown}

	t.Run("a tunnel that was never built settles at DOWN", func(t *testing.T) {
		read, reads := scriptedReader(
			securitycloud.GatewayStatusStatePending,
			securitycloud.GatewayStatusStateDown,
		)

		_, lastState, reached := waitForGatewayState(context.Background(), read, "a1b2", want, time.Millisecond, "", 0)

		if !reached {
			t.Fatalf("DOWN must satisfy an IPsec create wait, got lastState=%q", lastState)
		}
		if lastState != securitycloud.GatewayStatusStateDown {
			t.Errorf("lastState = %q, want DOWN", lastState)
		}
		if *reads != 2 {
			t.Errorf("reads = %d, want 2 — PENDING must not satisfy it", *reads)
		}
	})

	t.Run("a pre-staged concentrator settles at UP without ever reporting DOWN", func(t *testing.T) {
		read, _ := scriptedReader(
			securitycloud.GatewayStatusStatePending,
			securitycloud.GatewayStatusStateUp,
		)

		_, lastState, reached := waitForGatewayState(context.Background(), read, "a1b2", want, time.Millisecond, "", 0)

		if !reached || lastState != securitycloud.GatewayStatusStateUp {
			t.Errorf("UP must satisfy it too: reached=%v lastState=%q", reached, lastState)
		}
	})

	t.Run("the wait has nothing to abandon, since DOWN is an answer", func(t *testing.T) {
		if got := gatewayAbandonState(want); got != "" {
			t.Errorf("gatewayAbandonState = %q, want empty: abandoning DOWN would abandon a satisfied wait", got)
		}
	})
}

// TestGatewayAbandonState_OnlyAppliesToTheUpWait pins that a disable is not released
// by DOWN. Disabling the 2026-08-31 IPsec probe took it from DOWN through PENDING to
// DISABLED, so treating DOWN as a reason to give up there would abandon a disable that
// was working.
func TestGatewayAbandonState_OnlyAppliesToTheUpWait(t *testing.T) {
	if got := gatewayAbandonState([]string{securitycloud.GatewayStatusStateUp}); got != securitycloud.GatewayStatusStateDown {
		t.Errorf("a wait for UP must be released by a dwelling DOWN, got %q", got)
	}
	if got := gatewayAbandonState([]string{securitycloud.GatewayStatusStateDisabled}); got != "" {
		t.Errorf("a wait for DISABLED must not be released by DOWN, got %q", got)
	}
}

// TestGatewayWaitTarget_Gates pins what each configuration waits for.
//
// There is no form gate any more. An enabled IPsec gateway waits for UP like any
// other, and the DOWN dwell is what releases it early when its tunnel is not up —
// see gatewayAbandonState and TestWaitForGatewayState_DwellOnDown. Folding the IPsec
// case into the dwell removed the special case rather than adding one: the dwell has
// to exist anyway for an internet gateway that goes DOWN, and it handles both.
//
// `enabled` selects the target. Both transitions drift through PENDING, so both need
// waiting on; only an unknown value waits for nothing, having no target to aim at.
func TestGatewayWaitTarget_Gates(t *testing.T) {
	cases := []struct {
		name      string
		plan      GatewayResourceModel
		op        gatewayWaitOperation
		wantWait  bool
		wantState []string
	}{
		{
			name:      "an enabled internet gateway waits to come up",
			plan:      GatewayResourceModel{Enabled: types.BoolValue(true)},
			wantWait:  true,
			wantState: []string{securitycloud.GatewayStatusStateUp},
		},
		{
			name:      "a disabled internet gateway waits to report disabled",
			plan:      GatewayResourceModel{Enabled: types.BoolValue(false)},
			wantWait:  true,
			wantState: []string{securitycloud.GatewayStatusStateDisabled},
		},
		{
			name:      "creating an enabled ipsec gateway accepts DOWN, since UP is out of reach",
			plan:      GatewayResourceModel{Enabled: types.BoolValue(true), IPSec: &IPSecModel{}},
			op:        gatewayWaitCreate,
			wantWait:  true,
			wantState: []string{securitycloud.GatewayStatusStateUp, securitycloud.GatewayStatusStateDown},
		},
		{
			name:      "updating an enabled ipsec gateway does wait, since the tunnel may already be up",
			plan:      GatewayResourceModel{Enabled: types.BoolValue(true), IPSec: &IPSecModel{}},
			op:        gatewayWaitUpdate,
			wantWait:  true,
			wantState: []string{securitycloud.GatewayStatusStateUp},
		},
		{
			name:      "creating an enabled internet gateway waits: nothing external gates it",
			plan:      GatewayResourceModel{Enabled: types.BoolValue(true)},
			op:        gatewayWaitCreate,
			wantWait:  true,
			wantState: []string{securitycloud.GatewayStatusStateUp},
		},
		{
			name:      "a disabled ipsec gateway waits to report disabled, since disabling is Jamf-side",
			plan:      GatewayResourceModel{Enabled: types.BoolValue(false), IPSec: &IPSecModel{}},
			wantWait:  true,
			wantState: []string{securitycloud.GatewayStatusStateDisabled},
		},
		{
			name:     "an unknown enabled value waits for nothing rather than guessing a target",
			plan:     GatewayResourceModel{Enabled: types.BoolUnknown()},
			wantWait: false,
		},
		{
			name:      "a null enabled value waits to come up, matching the schema default",
			plan:      GatewayResourceModel{Enabled: types.BoolNull()},
			wantWait:  true,
			wantState: []string{securitycloud.GatewayStatusStateUp},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := tc.plan
			op := tc.op
			if op == (gatewayWaitOperation{}) {
				op = gatewayWaitUpdate
			}
			gotState, gotWait := gatewayWaitTarget(&plan, op)
			if gotWait != tc.wantWait {
				t.Fatalf("gatewayWaitTarget wait = %v, want %v", gotWait, tc.wantWait)
			}
			if gotWait && !slices.Equal(gotState, tc.wantState) {
				t.Errorf("gatewayWaitTarget state = %q, want %q", gotState, tc.wantState)
			}
		})
	}
}

// TestGatewayWaitTarget_DependsOnTheEnabledDefault pins the coupling that makes the
// null case above safe.
//
// The gate reads `enabled` with ValueBool(), which answers false for a null value as
// well as for an explicit false. That is only correct because the schema defaults
// `enabled` to true, so the framework has already replaced a null with true by the
// time Create or Update sees the plan and the gate never meets one. Nothing in the
// gate itself expresses that dependency, so removing the default — or flipping it to
// false — would silently stop every gateway that does not set `enabled` explicitly
// from waiting, and no other test here would fail.
//
// This test fails in that case. If it does, the fix is to decide what an absent
// `enabled` should mean and make the gate say so, not to update the expectation.
func TestGatewayWaitTarget_DependsOnTheEnabledDefault(t *testing.T) {
	s := resourceSchema(t)

	enabled, ok := s.Attributes["enabled"].(rschema.BoolAttribute)
	if !ok {
		t.Fatalf("enabled must be a BoolAttribute, got %T", s.Attributes["enabled"])
	}
	if enabled.Default == nil {
		t.Fatal("enabled must carry a schema default: gatewayWaitTarget treats a null value as enabled to match it")
	}

	var resp defaults.BoolResponse
	enabled.Default.DefaultBool(context.Background(), defaults.BoolRequest{}, &resp)
	if !resp.PlanValue.ValueBool() {
		t.Errorf("enabled defaults to %v; gatewayWaitTarget treats a null enabled as true, so an unset enabled would now wait for the wrong state", resp.PlanValue)
	}
}

// TestAppendGatewayWaitWarning_NamesTheStateReached pins that an exhausted wait is
// reported as a warning and never an error, and that the wording distinguishes the
// states, because they call for different responses.
//
// Never an error: by the time the wait is exhausted the gateway exists and is
// billable, and an error returned from a create the server accepted makes Terraform
// taint the resource and destroy and recreate it on the next apply. The warning-only
// assertion here is what keeps the resource in state — a diagnostic with no error in
// it cannot discard one.
func TestAppendGatewayWaitWarning_NamesTheStateReached(t *testing.T) {
	cases := []struct {
		name      string
		op        gatewayWaitOperation
		want      string
		lastState string
		wantText  []string
	}{
		{
			name:      "still provisioning reads as slowness on create",
			op:        gatewayWaitCreate,
			lastState: securitycloud.GatewayStatusStatePending,
			wantText: []string{
				"created successfully",
				"taking longer",
				"stalled",
				"settles on a later refresh",
				"`create`",
			},
		},
		{
			name:      "still provisioning reads as slowness on update",
			op:        gatewayWaitUpdate,
			lastState: securitycloud.GatewayStatusStatePending,
			wantText: []string{
				"updated successfully",
				"taking longer",
				"`update`",
			},
		},
		{
			name:      "down reads as a fault to investigate, not slowness",
			op:        gatewayWaitCreate,
			lastState: securitycloud.GatewayStatusStateDown,
			wantText: []string{
				"created successfully",
				"unreachable or degraded",
				"more likely a fault than slowness",
				"admin UI",
			},
		},
		{
			name:      "any other state is repeated verbatim",
			op:        gatewayWaitCreate,
			lastState: securitycloud.GatewayStatusStateDisabled,
			wantText: []string{
				securitycloud.GatewayStatusStateDisabled,
				"not a status the provider expects",
			},
		},
		{
			name:      "an unread status says so",
			op:        gatewayWaitCreate,
			lastState: "",
			wantText: []string{
				"could not read the gateway's status",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			appendGatewayWaitWarning(&diags, tc.op, []string{tc.want}, tc.lastState)

			if diags.HasError() {
				t.Fatalf("an exhausted wait must never error, or a billable gateway is tainted and replaced: %v", diags)
			}
			if diags.WarningsCount() != 1 {
				t.Fatalf("warnings = %d, want 1: %v", diags.WarningsCount(), diags)
			}
			warning := diags.Warnings()[0]
			text := warning.Summary() + " " + warning.Detail()
			for _, want := range tc.wantText {
				if !strings.Contains(text, want) {
					t.Errorf("warning %q does not mention %q", text, want)
				}
			}
			if strings.Contains(text, "dedicatedIps") || strings.Contains(text, "GET ") {
				t.Errorf("warning %q leaks wire vocabulary", text)
			}
		})
	}
}
