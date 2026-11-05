//go:build skipkeycanary

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// NamePrefix marks every object the canary creates. Nothing without it is ever deleted.
const NamePrefix = "zz-canary-"

// Kind is a PreStage family.
type Kind string

// The two PreStage families the canary probes.
const (
	Computer Kind = "computer"
	Mobile   Kind = "mobile"
)

// Kinds lists the families in report order.
var Kinds = []Kind{Computer, Mobile}

// ErrAlreadyRegistered is what API.UploadToken returns when Jamf Pro refuses the token because an
// instance already holds it.
var ErrAlreadyRegistered = errors.New("token already registered")

// Instance is an Automated Device Enrollment instance. ServerUUID is the identifier Apple gives
// the MDM server record the token belongs to, empty when Jamf Pro does not return one.
type Instance struct {
	ID         string
	Name       string
	ServerUUID string
}

// Prestage is a PreStage enrollment of either family.
type Prestage struct {
	ID   string
	Name string
}

// API is the slice of Jamf Pro the canary touches. The live adapter implements it over the SDK;
// the tests implement it with a fake.
type API interface {
	ListInstances(ctx context.Context) ([]Instance, error)
	UploadToken(ctx context.Context, name string, token []byte) (string, error)
	RenameInstance(ctx context.Context, id, name string) error
	WaitInstanceSynced(ctx context.Context, id string) error
	DeleteInstance(ctx context.Context, id string) error
	ListPrestages(ctx context.Context, kind Kind) ([]Prestage, error)
	CreatePrestage(ctx context.Context, kind Kind, name, instanceID string) (string, error)
	PrestageSkipKeys(ctx context.Context, kind Kind, id string) ([]string, error)
	DeletePrestage(ctx context.Context, kind Kind, id string) error
}

// Options configures one run. ServerUUID is optional: the server UUID of the instance that holds
// Token, which lets the canary pick that instance when Jamf Pro refuses the upload because the
// token is already registered.
type Options struct {
	DryRun         bool
	Token          []byte
	InstanceName   string
	ServerUUID     string
	CleanupTimeout time.Duration
	Logf           func(format string, args ...any)
}

// Outcome reports what a run observed and did.
type Outcome struct {
	Observed        map[Kind][]string
	CreatedInstance string
	ReusedInstance  string
	Swept           int
}

// Plan lists the leftovers a previous run could have abandoned.
type Plan struct {
	Prestages map[Kind][]Prestage
	Instances []Instance
}

// Empty reports whether there is nothing to sweep.
func (p Plan) Empty() bool {
	if len(p.Instances) > 0 {
		return false
	}
	for _, list := range p.Prestages {
		if len(list) > 0 {
			return false
		}
	}
	return true
}

// IsCanaryName reports whether a name belongs to the canary.
func IsCanaryName(name string) bool {
	return strings.HasPrefix(name, NamePrefix)
}

// Orphans selects the canary-named objects from what the tenant holds, and nothing else.
func Orphans(instances []Instance, prestages map[Kind][]Prestage) Plan {
	plan := Plan{Prestages: map[Kind][]Prestage{}}
	for _, kind := range Kinds {
		for _, p := range prestages[kind] {
			if IsCanaryName(p.Name) {
				plan.Prestages[kind] = append(plan.Prestages[kind], p)
			}
		}
	}
	for _, in := range instances {
		if IsCanaryName(in.Name) {
			plan.Instances = append(plan.Instances, in)
		}
	}
	return plan
}

// Deletable is the guard on instance deletion: only an id this run created, and that the tenant
// did not already hold, may be deleted.
func Deletable(id string, snapshot []Instance) bool {
	if id == "" {
		return false
	}
	for _, in := range snapshot {
		if in.ID == id {
			return false
		}
	}
	return true
}

// PickReusable chooses the instance to reuse when Jamf Pro refuses the upload because the tenant
// already holds the token. The refusal does not say which instance holds it, and a PreStage
// created on an unrelated instance pushes a DEP profile to another Apple organisation, so the
// choice is never a guess: exactly one instance must match the configured server UUID or the
// canary's own instance name, and anything else is an error that lists what the tenant holds.
func PickReusable(snapshot []Instance, serverUUID, name string) (Instance, error) {
	var matches []Instance
	for _, in := range snapshot {
		if (serverUUID != "" && in.ServerUUID == serverUUID) || (name != "" && in.Name == name) {
			matches = append(matches, in)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Instance{}, fmt.Errorf("upload refused as already registered, but no ADE instance matches the server UUID or the name %q; the tenant lists %s; set %s to the server UUID of the instance that holds the token", name, describeInstances(snapshot), serverUUIDVariable)
	default:
		return Instance{}, fmt.Errorf("upload refused as already registered, but %d ADE instances match the server UUID or the name %q (%s), so the canary cannot tell which one holds the token", len(matches), name, describeInstances(matches))
	}
}

func describeInstances(list []Instance) string {
	if len(list) == 0 {
		return "no ADE instance"
	}
	sorted := append([]Instance(nil), list...)
	sort.Slice(sorted, func(i, j int) bool {
		if len(sorted[i].ID) != len(sorted[j].ID) {
			return len(sorted[i].ID) < len(sorted[j].ID)
		}
		return sorted[i].ID < sorted[j].ID
	})
	parts := make([]string, 0, len(sorted))
	for _, in := range sorted {
		parts = append(parts, fmt.Sprintf("%s (%q)", in.ID, in.Name))
	}
	return strings.Join(parts, ", ")
}

// PrestageName is the throwaway name for a family.
func PrestageName(kind Kind) string {
	return NamePrefix + string(kind)
}

// Run executes one canary pass. It sweeps leftovers, makes sure an instance exists, creates and
// reads back one PreStage per family, and removes everything it made before returning. Cleanup
// runs on every exit path, including errors and a cancelled context.
func Run(ctx context.Context, api API, opts Options) (out Outcome, err error) {
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	out.Observed = map[Kind][]string{}

	instances, err := api.ListInstances(ctx)
	if err != nil {
		return out, fmt.Errorf("list ADE instances: %w", err)
	}
	prestages := map[Kind][]Prestage{}
	for _, kind := range Kinds {
		list, lerr := api.ListPrestages(ctx, kind)
		if lerr != nil {
			return out, fmt.Errorf("list %s prestages: %w", kind, lerr)
		}
		prestages[kind] = list
	}

	plan := Orphans(instances, prestages)
	if opts.DryRun {
		describePlan(logf, plan, instances, opts)
		return out, nil
	}

	if sweepErr := sweep(ctx, api, plan, logf); sweepErr != nil {
		return out, sweepErr
	}
	out.Swept = len(plan.Instances)
	for _, list := range plan.Prestages {
		out.Swept += len(list)
	}

	snapshot, err := api.ListInstances(ctx)
	if err != nil {
		return out, fmt.Errorf("snapshot ADE instances: %w", err)
	}

	var createdPrestages []createdPrestage
	var uploadUncertain bool
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.CleanupTimeout)
		defer cancel()
		var aerr error
		if uploadUncertain && out.CreatedInstance == "" {
			var stray string
			stray, aerr = adoptStray(cleanupCtx, api, snapshot, logf)
			out.CreatedInstance = stray
		}
		cerr := cleanup(cleanupCtx, api, createdPrestages, out.CreatedInstance, snapshot, logf)
		err = errors.Join(err, aerr, cerr)
	}()

	res, err := ensureInstance(ctx, api, opts, snapshot, logf)
	out.CreatedInstance = res.created
	uploadUncertain = res.uncertain
	if err != nil {
		return out, err
	}
	instanceID := res.id
	if res.created == "" {
		out.ReusedInstance = instanceID
	}

	for _, kind := range Kinds {
		id, cerr := api.CreatePrestage(ctx, kind, PrestageName(kind), instanceID)
		if cerr != nil {
			return out, fmt.Errorf("create %s prestage: %w", kind, cerr)
		}
		createdPrestages = append(createdPrestages, createdPrestage{kind: kind, id: id})
		logf("created %s prestage %s on instance %s", kind, id, instanceID)

		keys, kerr := api.PrestageSkipKeys(ctx, kind, id)
		if kerr != nil {
			return out, fmt.Errorf("read %s prestage %s: %w", kind, id, kerr)
		}
		if len(keys) == 0 {
			return out, fmt.Errorf("%s prestage %s returned no skipSetupItems keys", kind, id)
		}
		sort.Strings(keys)
		out.Observed[kind] = keys
	}
	return out, nil
}

type createdPrestage struct {
	kind Kind
	id   string
}

func describePlan(logf func(string, ...any), plan Plan, instances []Instance, opts Options) {
	logf("dry run: nothing will be changed")
	logf("tenant holds %d ADE instance(s); canary-named leftovers are the only objects eligible for deletion", len(instances))
	for _, kind := range Kinds {
		for _, p := range plan.Prestages[kind] {
			logf("would sweep %s prestage %s (%s)", kind, p.ID, p.Name)
		}
	}
	for _, in := range plan.Instances {
		logf("would sweep ADE instance %s (%s)", in.ID, in.Name)
	}
	if plan.Empty() {
		logf("no leftovers to sweep")
	}
	logf("would upload the ADE token as %q; if the tenant already holds it, an existing instance is reused and never deleted", opts.InstanceName)
	for _, kind := range Kinds {
		logf("would create, read back and delete one %s prestage named %q", kind, PrestageName(kind))
	}
	logf("would delete the ADE instance only if this run created it")
}

func sweep(ctx context.Context, api API, plan Plan, logf func(string, ...any)) error {
	var errs []error
	for _, kind := range Kinds {
		for _, p := range plan.Prestages[kind] {
			if !IsCanaryName(p.Name) {
				continue
			}
			logf("sweeping %s prestage %s (%s)", kind, p.ID, p.Name)
			if err := api.DeletePrestage(ctx, kind, p.ID); err != nil {
				errs = append(errs, fmt.Errorf("sweep %s prestage %s: %w", kind, p.ID, err))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	for _, in := range plan.Instances {
		if !IsCanaryName(in.Name) {
			continue
		}
		logf("sweeping ADE instance %s (%s)", in.ID, in.Name)
		if err := api.DeleteInstance(ctx, in.ID); err != nil {
			errs = append(errs, fmt.Errorf("sweep ADE instance %s: %w", in.ID, err))
		}
	}
	return errors.Join(errs...)
}

// instanceResult is what ensureInstance learned. id is the instance to put PreStages on, created
// the id of an instance this run made and must delete, and uncertain says the upload failed in a
// way that may have created an instance the run could not identify.
type instanceResult struct {
	id        string
	created   string
	uncertain bool
}

// ensureInstance records the id of a new instance the moment the upload returns it, before the
// rename and the sync wait, so a failure in either step still leaves the instance in the created
// field for cleanup. When the upload errors or returns no id the request may still have reached
// Jamf Pro, so it looks for instances absent from the pre-upload snapshot. A server-named
// instance that this run created would otherwise be invisible to the canary-name sweep.
func ensureInstance(ctx context.Context, api API, opts Options, snapshot []Instance, logf func(string, ...any)) (instanceResult, error) {
	uploaded, uerr := api.UploadToken(ctx, opts.InstanceName, opts.Token)
	switch {
	case errors.Is(uerr, ErrAlreadyRegistered):
		reuse, err := PickReusable(snapshot, opts.ServerUUID, opts.InstanceName)
		if err != nil {
			return instanceResult{}, err
		}
		logf("token already registered; reusing ADE instance %s (%s) and leaving it untouched", reuse.ID, reuse.Name)
		return instanceResult{id: reuse.ID}, nil
	case uerr != nil:
		res, rerr := recoverUpload(ctx, api, opts, snapshot, logf)
		return res, errors.Join(fmt.Errorf("upload ADE token: %w", uerr), rerr)
	case uploaded == "":
		res, rerr := recoverUpload(ctx, api, opts, snapshot, logf)
		return res, errors.Join(errors.New("upload ADE token: response carried no instance id"), rerr)
	case !Deletable(uploaded, snapshot):
		logf("upload resolved to existing ADE instance %s; reusing it and leaving it untouched", uploaded)
		return instanceResult{id: uploaded}, nil
	}

	logf("uploaded ADE token as instance %s", uploaded)
	if err := api.RenameInstance(ctx, uploaded, opts.InstanceName); err != nil {
		return instanceResult{created: uploaded}, fmt.Errorf("rename ADE instance %s: %w", uploaded, err)
	}
	if err := api.WaitInstanceSynced(ctx, uploaded); err != nil {
		return instanceResult{created: uploaded}, fmt.Errorf("wait for ADE sync on instance %s: %w", uploaded, err)
	}
	return instanceResult{id: uploaded, created: uploaded}, nil
}

func recoverUpload(ctx context.Context, api API, opts Options, snapshot []Instance, logf func(string, ...any)) (instanceResult, error) {
	listCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.CleanupTimeout)
	defer cancel()
	stray, err := adoptStray(listCtx, api, snapshot, logf)
	return instanceResult{created: stray, uncertain: err != nil}, err
}

// adoptStray finds the instance a failed upload left behind: one the tenant holds now and the
// pre-upload snapshot did not. The shared concurrency group is what makes "absent from the
// snapshot" mean "created by this run". It deletes nothing itself and returns no id when two or
// more instances appeared, since it cannot tell which one belongs to this run.
func adoptStray(ctx context.Context, api API, snapshot []Instance, logf func(string, ...any)) (string, error) {
	instances, err := api.ListInstances(ctx)
	if err != nil {
		return "", fmt.Errorf("list ADE instances after the failed upload: %w", err)
	}
	var strays []Instance
	for _, in := range instances {
		if Deletable(in.ID, snapshot) {
			strays = append(strays, in)
		}
	}
	switch len(strays) {
	case 0:
		return "", nil
	case 1:
		logf("the failed upload left ADE instance %s (%s); this run created it and will delete it", strays[0].ID, strays[0].Name)
		return strays[0].ID, nil
	default:
		return "", fmt.Errorf("the failed upload coincided with %d new ADE instances (%s), so the canary cannot tell which one it created and deleted none", len(strays), describeInstances(strays))
	}
}

func cleanup(ctx context.Context, api API, made []createdPrestage, createdInstance string, snapshot []Instance, logf func(string, ...any)) error {
	var errs []error
	for _, m := range slices.Backward(made) {

		logf("deleting %s prestage %s", m.kind, m.id)
		if err := api.DeletePrestage(ctx, m.kind, m.id); err != nil {
			errs = append(errs, fmt.Errorf("delete %s prestage %s: %w", m.kind, m.id, err))
		}
	}
	if Deletable(createdInstance, snapshot) {
		logf("deleting ADE instance %s created by this run", createdInstance)
		if err := api.DeleteInstance(ctx, createdInstance); err != nil {
			errs = append(errs, fmt.Errorf("delete ADE instance %s: %w", createdInstance, err))
		}
	}
	if residue := findResidue(ctx, api, createdInstance, snapshot); len(residue) > 0 {
		errs = append(errs, fmt.Errorf("canary residue remains: %s", strings.Join(residue, "; ")))
	}
	return errors.Join(errs...)
}

func findResidue(ctx context.Context, api API, createdInstance string, snapshot []Instance) []string {
	var residue []string
	for _, kind := range Kinds {
		list, err := api.ListPrestages(ctx, kind)
		if err != nil {
			residue = append(residue, fmt.Sprintf("could not re-list %s prestages: %v", kind, err))
			continue
		}
		for _, p := range list {
			if IsCanaryName(p.Name) {
				residue = append(residue, fmt.Sprintf("%s prestage %s (%s)", kind, p.ID, p.Name))
			}
		}
	}
	instances, err := api.ListInstances(ctx)
	if err != nil {
		return append(residue, fmt.Sprintf("could not re-list ADE instances: %v", err))
	}
	for _, in := range instances {
		if IsCanaryName(in.Name) || (Deletable(createdInstance, snapshot) && in.ID == createdInstance) {
			residue = append(residue, fmt.Sprintf("ADE instance %s (%s)", in.ID, in.Name))
		}
	}
	return residue
}
