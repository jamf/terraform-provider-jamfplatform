// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package managed_software_updates

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// fakeToggleClient is a hand-rolled toggleClient that scripts the async settle: each GET
// returns getValues[i] (clamped to the last entry once exhausted), so the test can model
// "PUT echo stale, GET catches up after N polls". getErr, when set, fails every GET.
type fakeToggleClient struct {
	getValues []bool
	getCalls  int
	getErr    error

	updateErr error
	updated   *pro.ManagedSoftwareUpdatePlanToggle

	status *pro.ManagedSoftwareUpdatePlanToggleStatusWrapper
}

func (f *fakeToggleClient) GetManagedSoftwareUpdateFeatureToggleV1(_ context.Context) (*pro.ManagedSoftwareUpdatePlanToggle, error) {
	if f.getErr != nil {
		f.getCalls++
		return nil, f.getErr
	}
	i := f.getCalls
	if i >= len(f.getValues) {
		i = len(f.getValues) - 1
	}
	f.getCalls++
	return &pro.ManagedSoftwareUpdatePlanToggle{Toggle: f.getValues[i]}, nil
}

func (f *fakeToggleClient) UpdateManagedSoftwareUpdateFeatureToggleV1(_ context.Context, request *pro.ManagedSoftwareUpdatePlanToggle) (*pro.ManagedSoftwareUpdatePlanToggle, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.updated = request
	// Echo a STALE value (the opposite of what was requested) to prove the poll ignores the
	// PUT response and trusts the GET instead.
	return &pro.ManagedSoftwareUpdatePlanToggle{Toggle: !request.Toggle}, nil
}

func (f *fakeToggleClient) GetManagedSoftwareUpdateFeatureToggleStatusV1(_ context.Context) (*pro.ManagedSoftwareUpdatePlanToggleStatusWrapper, error) {
	return f.status, nil
}

// TestApplyAndSettle_ConvergesAfterStalePUT proves the apply ignores the stale PUT echo and
// returns the settled value once GET catches up.
func TestApplyAndSettle_ConvergesAfterStalePUT(t *testing.T) {
	f := &fakeToggleClient{getValues: []bool{false, false, true}} // settles to true on the 3rd GET

	got, err := applyAndSettle(context.Background(), f, &pro.ManagedSoftwareUpdatePlanToggle{Toggle: true}, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || !got.Toggle {
		t.Fatalf("expected settled toggle=true, got %+v", got)
	}
	if f.updated == nil || !f.updated.Toggle {
		t.Errorf("expected PUT to carry toggle=true, got %+v", f.updated)
	}
}

// TestPollToggleSettled_NoOpReturnsOnFirstGet proves the adopt/no-op case (desired already
// equals current) returns immediately without waiting a tick.
func TestPollToggleSettled_NoOpReturnsOnFirstGet(t *testing.T) {
	f := &fakeToggleClient{getValues: []bool{true}}

	got, err := pollToggleSettled(context.Background(), f, true, time.Hour) // long interval — must not be hit
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || !got.Toggle {
		t.Fatalf("expected toggle=true on first GET, got %+v", got)
	}
	if f.getCalls != 1 {
		t.Errorf("expected exactly 1 GET, got %d", f.getCalls)
	}
}

// TestPollToggleSettled_TimeoutNamesAbandonAction proves a never-converging toggle yields a
// timeout error that points the user at the break-glass abandon action, enriched with the
// last reported background status.
func TestPollToggleSettled_TimeoutNamesAbandonAction(t *testing.T) {
	f := &fakeToggleClient{
		getValues: []bool{false}, // never reaches desired=true
		status: &pro.ManagedSoftwareUpdatePlanToggleStatusWrapper{
			ToggleOn: &pro.ManagedSoftwareUpdatePlanToggleStatus{State: "RUNNING", ExitState: "", ExitMessage: "still working"},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := pollToggleSettled(ctx, f, true, time.Millisecond)
	if err == nil {
		t.Fatal("expected a settle-timeout error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "jamfplatform_pro_managed_software_update_abandon") {
		t.Errorf("error should name the abandon action, got: %s", msg)
	}
	if !strings.Contains(msg, "RUNNING") {
		t.Errorf("error should include the last reported status, got: %s", msg)
	}
}

// TestPollToggleSettled_TimeoutSurfacesGetError proves a persistent GET failure is surfaced
// rather than masked behind a generic settle-timeout message (no 4xx hidden as a timeout).
func TestPollToggleSettled_TimeoutSurfacesGetError(t *testing.T) {
	sentinel := errors.New("403 INVALID_TOKEN")
	f := &fakeToggleClient{getErr: sentinel}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := pollToggleSettled(ctx, f, true, time.Millisecond)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected the underlying GET error to be surfaced, got: %v", err)
	}
}

// TestApplyAndSettle_UpdateErrorPropagates proves a failed PUT short-circuits before polling.
func TestApplyAndSettle_UpdateErrorPropagates(t *testing.T) {
	sentinel := errors.New("500 boom")
	f := &fakeToggleClient{updateErr: sentinel}

	_, err := applyAndSettle(context.Background(), f, &pro.ManagedSoftwareUpdatePlanToggle{Toggle: true}, time.Millisecond)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected PUT error to propagate, got: %v", err)
	}
	if f.getCalls != 0 {
		t.Errorf("expected no GET after a failed PUT, got %d", f.getCalls)
	}
}
