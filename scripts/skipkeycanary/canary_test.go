//go:build skipkeycanary

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeAPI struct {
	instances    []Instance
	prestages    map[Kind][]Prestage
	uploadID     string
	uploadErr    error
	createErr    map[Kind]error
	skipKeys     map[Kind][]string
	deleteErr    map[string]error
	deletedInst  []string
	deletedPre   []string
	renamed      map[string]string
	uploads      int
	nextPrestage int
	leakOnDelete bool
	renameErr    error
	listCalls    int
	listFailOn   map[int]bool
	lostUploads  []string
	uploadNoID   bool
}

const serverAssignedName = "Apple Business Manager"

func newFake() *fakeAPI {
	return &fakeAPI{
		prestages:  map[Kind][]Prestage{},
		createErr:  map[Kind]error{},
		deleteErr:  map[string]error{},
		renamed:    map[string]string{},
		listFailOn: map[int]bool{},
		skipKeys:   map[Kind][]string{Computer: {"B", "A"}, Mobile: {"C"}},
	}
}

func (f *fakeAPI) ListInstances(context.Context) ([]Instance, error) {
	f.listCalls++
	if f.listFailOn[f.listCalls] {
		return nil, errors.New("tenant unreachable")
	}
	return append([]Instance(nil), f.instances...), nil
}

func (f *fakeAPI) UploadToken(_ context.Context, _ string, _ []byte) (string, error) {
	f.uploads++
	for _, id := range f.lostUploads {
		f.instances = append(f.instances, Instance{ID: id, Name: serverAssignedName})
	}
	if f.uploadErr != nil {
		return "", f.uploadErr
	}
	for _, in := range f.instances {
		if in.ID == f.uploadID {
			return f.uploadID, nil
		}
	}
	f.instances = append(f.instances, Instance{ID: f.uploadID, Name: serverAssignedName})
	if f.uploadNoID {
		return "", nil
	}
	return f.uploadID, nil
}

func (f *fakeAPI) RenameInstance(_ context.Context, id, name string) error {
	if f.renameErr != nil {
		return f.renameErr
	}
	f.renamed[id] = name
	for i := range f.instances {
		if f.instances[i].ID == id {
			f.instances[i].Name = name
		}
	}
	return nil
}

func (f *fakeAPI) WaitInstanceSynced(context.Context, string) error { return nil }

func (f *fakeAPI) DeleteInstance(_ context.Context, id string) error {
	if err := f.deleteErr["instance:"+id]; err != nil {
		return err
	}
	f.deletedInst = append(f.deletedInst, id)
	if f.leakOnDelete {
		return nil
	}
	kept := f.instances[:0]
	for _, in := range f.instances {
		if in.ID != id {
			kept = append(kept, in)
		}
	}
	f.instances = kept
	return nil
}

func (f *fakeAPI) ListPrestages(_ context.Context, kind Kind) ([]Prestage, error) {
	return append([]Prestage(nil), f.prestages[kind]...), nil
}

func (f *fakeAPI) CreatePrestage(_ context.Context, kind Kind, name, _ string) (string, error) {
	if err := f.createErr[kind]; err != nil {
		return "", err
	}
	f.nextPrestage++
	id := fmt.Sprintf("p%d", f.nextPrestage)
	f.prestages[kind] = append(f.prestages[kind], Prestage{ID: id, Name: name})
	return id, nil
}

func (f *fakeAPI) PrestageSkipKeys(_ context.Context, kind Kind, _ string) ([]string, error) {
	return append([]string(nil), f.skipKeys[kind]...), nil
}

func (f *fakeAPI) DeletePrestage(_ context.Context, kind Kind, id string) error {
	if err := f.deleteErr["prestage:"+id]; err != nil {
		return err
	}
	f.deletedPre = append(f.deletedPre, id)
	kept := f.prestages[kind][:0]
	for _, p := range f.prestages[kind] {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	f.prestages[kind] = kept
	return nil
}

func opts() Options {
	return Options{Token: []byte("t"), InstanceName: NamePrefix + "ade", CleanupTimeout: time.Second}
}

func TestOrphansSelectsOnlyCanaryNames(t *testing.T) {
	plan := Orphans(
		[]Instance{{ID: "1", Name: "Apple Business Manager"}, {ID: "9", Name: "zz-canary-ade"}},
		map[Kind][]Prestage{
			Computer: {{ID: "3", Name: "Shared"}, {ID: "7", Name: "zz-canary-computer"}},
			Mobile:   {{ID: "2", Name: "zz-canaryish"}, {ID: "5", Name: "zz-canary-mobile"}},
		},
	)
	if len(plan.Instances) != 1 || plan.Instances[0].ID != "9" {
		t.Fatalf("instances = %+v", plan.Instances)
	}
	if got := plan.Prestages[Computer]; len(got) != 1 || got[0].ID != "7" {
		t.Fatalf("computer = %+v", got)
	}
	if got := plan.Prestages[Mobile]; len(got) != 1 || got[0].ID != "5" {
		t.Fatalf("mobile = %+v", got)
	}
}

func TestOrphansEmptyTenant(t *testing.T) {
	if !Orphans(nil, nil).Empty() {
		t.Fatal("empty tenant must have nothing to sweep")
	}
}

func TestDeletableGuard(t *testing.T) {
	snapshot := []Instance{{ID: "1"}}
	cases := []struct {
		id   string
		want bool
	}{{"", false}, {"1", false}, {"2", true}}
	for _, c := range cases {
		if got := Deletable(c.id, snapshot); got != c.want {
			t.Errorf("Deletable(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}

func TestPickReusable(t *testing.T) {
	tenant := []Instance{{ID: "12", Name: "Other", ServerUUID: "u-other"}, {ID: "3", Name: "ABM", ServerUUID: "u-abm"}}
	cases := []struct {
		name     string
		snapshot []Instance
		uuid     string
		match    string
		wantID   string
		wantErr  string
	}{
		{"matches the server uuid", tenant, "u-abm", "zz-canary-ade", "3", ""},
		{"matches the instance name", tenant, "", "Other", "12", ""},
		{"no match lists what the tenant holds", tenant, "u-missing", "zz-canary-ade", "", `3 ("ABM"), 12 ("Other")`},
		{"no match names the variable to set", tenant, "", "zz-canary-ade", "", serverUUIDVariable},
		{"empty tenant", nil, "u-abm", "zz-canary-ade", "", "no ADE instance"},
		{"two instances match", tenant, "u-abm", "Other", "", "cannot tell which one holds the token"},
		{"an empty uuid never matches an instance without one", []Instance{{ID: "1", Name: "x"}}, "", "zz-canary-ade", "", "no ADE instance matches"},
	}
	for _, c := range cases {
		got, err := PickReusable(c.snapshot, c.uuid, c.match)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%s: err = %v, want it to contain %q", c.name, err, c.wantErr)
			}
			continue
		}
		if err != nil || got.ID != c.wantID {
			t.Errorf("%s: got %+v err=%v, want id %s", c.name, got, err, c.wantID)
		}
	}
}

func TestRunOnEmptyTenantUploadsAndCleansUp(t *testing.T) {
	f := newFake()
	f.uploadID = "50"
	out, err := Run(context.Background(), f, opts())
	if err != nil {
		t.Fatal(err)
	}
	if out.CreatedInstance != "50" || out.ReusedInstance != "" {
		t.Fatalf("outcome = %+v", out)
	}
	if !reflect.DeepEqual(out.Observed[Computer], []string{"A", "B"}) {
		t.Fatalf("observed computer = %v", out.Observed[Computer])
	}
	if !reflect.DeepEqual(f.deletedInst, []string{"50"}) {
		t.Fatalf("deleted instances = %v", f.deletedInst)
	}
	if len(f.deletedPre) != 2 || len(f.instances) != 0 || len(f.prestages[Computer])+len(f.prestages[Mobile]) != 0 {
		t.Fatalf("residue: %+v %+v", f.instances, f.prestages)
	}
	if f.renamed["50"] != NamePrefix+"ade" {
		t.Fatalf("instance not renamed: %v", f.renamed)
	}
}

func TestRunDeletesPrestagesBeforeTheInstance(t *testing.T) {
	var order []string
	f := newFake()
	f.uploadID = "50"
	wrap := &orderAPI{fakeAPI: f, order: &order}
	if _, err := Run(context.Background(), wrap, opts()); err != nil {
		t.Fatal(err)
	}
	want := []string{"prestage", "prestage", "instance"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

type orderAPI struct {
	*fakeAPI
	order *[]string
}

func (o *orderAPI) DeletePrestage(ctx context.Context, kind Kind, id string) error {
	*o.order = append(*o.order, "prestage")
	return o.fakeAPI.DeletePrestage(ctx, kind, id)
}

func (o *orderAPI) DeleteInstance(ctx context.Context, id string) error {
	*o.order = append(*o.order, "instance")
	return o.fakeAPI.DeleteInstance(ctx, id)
}

func TestRunNeverDeletesAnInstanceTheTenantAlreadyHeld(t *testing.T) {
	f := newFake()
	f.instances = []Instance{{ID: "1", Name: "Apple Business Manager"}}
	f.uploadID = "1"
	f.uploadErr = nil
	out, err := Run(context.Background(), f, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.deletedInst) != 0 {
		t.Fatalf("deleted instances = %v", f.deletedInst)
	}
	if out.ReusedInstance != "1" || out.CreatedInstance != "" {
		t.Fatalf("outcome = %+v", out)
	}
	if len(f.renamed) != 0 {
		t.Fatalf("a reused instance must not be renamed: %v", f.renamed)
	}
}

func TestRunReusesTheInstanceMatchingTheServerUUIDWhenTheUploadIsRefused(t *testing.T) {
	f := newFake()
	f.instances = []Instance{{ID: "4", Name: "ABM", ServerUUID: "u-abm"}, {ID: "2", Name: "Other", ServerUUID: "u-other"}}
	f.uploadErr = fmt.Errorf("%w: 409", ErrAlreadyRegistered)
	o := opts()
	o.ServerUUID = "u-abm"
	out, err := Run(context.Background(), f, o)
	if err != nil {
		t.Fatal(err)
	}
	if out.ReusedInstance != "4" || len(f.deletedInst) != 0 {
		t.Fatalf("outcome = %+v deleted = %v", out, f.deletedInst)
	}
	if len(f.prestages[Computer])+len(f.prestages[Mobile]) != 0 {
		t.Fatalf("prestages left behind: %+v", f.prestages)
	}
}

func TestRunRefusedAsRegisteredWithNoMatchFailsWithoutTouchingAnything(t *testing.T) {
	f := newFake()
	f.instances = []Instance{{ID: "4", Name: "ABM", ServerUUID: "u-abm"}, {ID: "2", Name: "Other"}}
	f.uploadErr = fmt.Errorf("%w: 409", ErrAlreadyRegistered)
	_, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), `2 ("Other"), 4 ("ABM")`) {
		t.Fatalf("err = %v", err)
	}
	if len(f.deletedInst) != 0 || len(f.prestages[Computer])+len(f.prestages[Mobile]) != 0 || f.nextPrestage != 0 {
		t.Fatalf("a refused, unmatched run changed the tenant: deleted=%v created=%d", f.deletedInst, f.nextPrestage)
	}
}

func TestRunRefusedAsRegisteredWithTwoMatchesFails(t *testing.T) {
	f := newFake()
	f.instances = []Instance{{ID: "4", Name: "ABM", ServerUUID: "u-abm"}, {ID: "2", Name: "Other", ServerUUID: "u-abm"}}
	f.uploadErr = fmt.Errorf("%w: 409", ErrAlreadyRegistered)
	o := opts()
	o.ServerUUID = "u-abm"
	_, err := Run(context.Background(), f, o)
	if err == nil || !strings.Contains(err.Error(), "cannot tell which one holds the token") {
		t.Fatalf("err = %v", err)
	}
	if len(f.deletedInst) != 0 || f.nextPrestage != 0 {
		t.Fatalf("an ambiguous run changed the tenant: deleted=%v created=%d", f.deletedInst, f.nextPrestage)
	}
}

func TestRunRefusedAsRegisteredOnEmptyTenantFails(t *testing.T) {
	f := newFake()
	f.uploadErr = ErrAlreadyRegistered
	if _, err := Run(context.Background(), f, opts()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRunSweepsLeftoversBeforeUploading(t *testing.T) {
	f := newFake()
	f.uploadID = "60"
	f.instances = []Instance{{ID: "1", Name: "Apple Business Manager"}, {ID: "9", Name: NamePrefix + "ade"}}
	f.prestages[Computer] = []Prestage{{ID: "3", Name: "Shared"}, {ID: "8", Name: NamePrefix + "computer"}}
	out, err := Run(context.Background(), f, opts())
	if err != nil {
		t.Fatal(err)
	}
	if out.Swept != 2 {
		t.Fatalf("swept = %d", out.Swept)
	}
	if !contains(f.deletedPre, "8") || contains(f.deletedPre, "3") {
		t.Fatalf("deleted prestages = %v", f.deletedPre)
	}
	if !contains(f.deletedInst, "9") || contains(f.deletedInst, "1") {
		t.Fatalf("deleted instances = %v", f.deletedInst)
	}
	if len(f.instances) != 1 || f.instances[0].ID != "1" {
		t.Fatalf("instances = %+v", f.instances)
	}
	if len(f.prestages[Computer]) != 1 || f.prestages[Computer][0].ID != "3" {
		t.Fatalf("prestages = %+v", f.prestages[Computer])
	}
}

func TestRunCleansUpWhenAPrestageCreateFails(t *testing.T) {
	f := newFake()
	f.uploadID = "50"
	f.createErr[Mobile] = errors.New("boom")
	_, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "create mobile prestage") {
		t.Fatalf("err = %v", err)
	}
	if len(f.prestages[Computer]) != 0 || len(f.instances) != 0 {
		t.Fatalf("residue after failure: %+v %+v", f.prestages, f.instances)
	}
}

func TestRunCleansUpWhenTheContextIsCancelled(t *testing.T) {
	f := newFake()
	f.uploadID = "50"
	ctx, cancel := context.WithCancel(context.Background())
	api := &cancelAfterUpload{fakeAPI: f, cancel: cancel}
	_, _ = Run(ctx, api, opts())
	if len(f.instances) != 0 {
		t.Fatalf("instance left behind: %+v", f.instances)
	}
}

type cancelAfterUpload struct {
	*fakeAPI
	cancel context.CancelFunc
}

func (c *cancelAfterUpload) RenameInstance(ctx context.Context, id, name string) error {
	err := c.fakeAPI.RenameInstance(ctx, id, name)
	c.cancel()
	return err
}

func (c *cancelAfterUpload) WaitInstanceSynced(ctx context.Context, _ string) error {
	return ctx.Err()
}

func TestRunFailsLoudlyOnResidue(t *testing.T) {
	f := newFake()
	f.uploadID = "50"
	f.leakOnDelete = true
	_, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "canary residue remains") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunReportsAFailedCleanup(t *testing.T) {
	f := newFake()
	f.uploadID = "50"
	f.deleteErr["instance:50"] = errors.New("locked")
	_, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunDeletesTheInstanceWhenTheRenameFails(t *testing.T) {
	f := newFake()
	f.uploadID = "50"
	f.renameErr = errors.New("rename refused")
	out, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "rename refused") {
		t.Fatalf("err = %v", err)
	}
	if out.CreatedInstance != "50" || !reflect.DeepEqual(f.deletedInst, []string{"50"}) || len(f.instances) != 0 {
		t.Fatalf("outcome = %+v deleted = %v instances = %+v", out, f.deletedInst, f.instances)
	}
}

func TestRunDeletesTheInstanceWhenTheUploadResponseIsLost(t *testing.T) {
	f := newFake()
	f.lostUploads = []string{"77"}
	f.uploadErr = errors.New("context deadline exceeded")
	out, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "upload ADE token") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "residue") {
		t.Fatalf("the stray instance was not cleaned up: %v", err)
	}
	if out.CreatedInstance != "77" || !reflect.DeepEqual(f.deletedInst, []string{"77"}) || len(f.instances) != 0 {
		t.Fatalf("outcome = %+v deleted = %v instances = %+v", out, f.deletedInst, f.instances)
	}
}

func TestRunDeletesTheInstanceWhenTheUploadReturnsNoID(t *testing.T) {
	f := newFake()
	f.uploadID = "78"
	f.uploadNoID = true
	out, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "no instance id") {
		t.Fatalf("err = %v", err)
	}
	if out.CreatedInstance != "78" || !reflect.DeepEqual(f.deletedInst, []string{"78"}) {
		t.Fatalf("outcome = %+v deleted = %v", out, f.deletedInst)
	}
}

func TestRunWithAnUploadErrorAndNoNewInstanceDeletesNothing(t *testing.T) {
	f := newFake()
	f.instances = []Instance{{ID: "1", Name: serverAssignedName}}
	f.uploadErr = errors.New("service unavailable")
	out, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "service unavailable") {
		t.Fatalf("err = %v", err)
	}
	if out.CreatedInstance != "" || len(f.deletedInst) != 0 || len(f.instances) != 1 {
		t.Fatalf("outcome = %+v deleted = %v instances = %+v", out, f.deletedInst, f.instances)
	}
}

func TestRunNeverDeletesAnInstanceTheSnapshotHeldAfterAFailedUpload(t *testing.T) {
	f := newFake()
	f.instances = []Instance{{ID: "1", Name: serverAssignedName}, {ID: "2", Name: "Other"}}
	f.lostUploads = []string{"90"}
	f.uploadErr = errors.New("connection reset")
	if _, err := Run(context.Background(), f, opts()); err == nil {
		t.Fatal("expected an error")
	}
	if !reflect.DeepEqual(f.deletedInst, []string{"90"}) {
		t.Fatalf("deleted = %v, want only the instance this run created", f.deletedInst)
	}
	if len(f.instances) != 2 {
		t.Fatalf("instances = %+v", f.instances)
	}
}

func TestRunDeletesNothingWhenSeveralNewInstancesAppearAfterAFailedUpload(t *testing.T) {
	f := newFake()
	f.lostUploads = []string{"90", "91"}
	f.uploadErr = errors.New("connection reset")
	_, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "cannot tell which one it created") {
		t.Fatalf("err = %v", err)
	}
	if len(f.deletedInst) != 0 {
		t.Fatalf("deleted = %v, want none", f.deletedInst)
	}
}

func TestCleanupRetriesTheStrayLookupWhenTheFirstListFails(t *testing.T) {
	f := newFake()
	f.lostUploads = []string{"77"}
	f.uploadErr = errors.New("context deadline exceeded")
	f.listFailOn[3] = true
	out, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "list ADE instances after the failed upload") {
		t.Fatalf("err = %v", err)
	}
	if out.CreatedInstance != "77" || !reflect.DeepEqual(f.deletedInst, []string{"77"}) || len(f.instances) != 0 {
		t.Fatalf("outcome = %+v deleted = %v instances = %+v", out, f.deletedInst, f.instances)
	}
}

func TestRunRejectsAnEmptyKeySet(t *testing.T) {
	f := newFake()
	f.uploadID = "50"
	f.skipKeys[Computer] = nil
	_, err := Run(context.Background(), f, opts())
	if err == nil || !strings.Contains(err.Error(), "no skipSetupItems keys") {
		t.Fatalf("err = %v", err)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	f := newFake()
	f.uploadID = "50"
	f.instances = []Instance{{ID: "9", Name: NamePrefix + "ade"}}
	f.prestages[Mobile] = []Prestage{{ID: "8", Name: NamePrefix + "mobile"}}
	var lines []string
	o := opts()
	o.DryRun = true
	o.Logf = func(format string, a ...any) { lines = append(lines, fmt.Sprintf(format, a...)) }
	if _, err := Run(context.Background(), f, o); err != nil {
		t.Fatal(err)
	}
	if f.uploads != 0 || len(f.deletedInst)+len(f.deletedPre) != 0 {
		t.Fatalf("dry run mutated the tenant: uploads=%d deleted=%v %v", f.uploads, f.deletedInst, f.deletedPre)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"would sweep mobile prestage 8", "would sweep ADE instance 9", "would upload"} {
		if !strings.Contains(joined, want) {
			t.Errorf("dry run output missing %q:\n%s", want, joined)
		}
	}
}

func contains(list []string, s string) bool {
	return slices.Contains(list, s)
}
