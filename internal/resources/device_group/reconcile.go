// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// reconcileAttempts and reconcileInterval bound how long a failed create waits for
// the group it may have made to become listable. A group a create is still
// building appeared in a list at 3.4s to 5.1s, and the failures worth
// reconciling return after 15s or more, so one look usually suffices; the extra
// attempts cover the fast 5xx that return in 0.1s to 0.3s while the service is
// still working. Variables so a test does not wait.
var (
	reconcileAttempts = 3
	reconcileInterval = 2 * time.Second
)

// namedGroupSnapshot records which groups already held the requested name and
// device type before a create was sent. It exists to tell a name collision that
// was there all along (a real 422, nothing to adopt) from one the create itself
// caused. Name is unique per device type, so any same-name group that appears
// after a clean snapshot came from this request.
//
// usable is false when the snapshot could not be taken. Reconciliation is then
// off, because without a "before" every same-name group is indistinguishable
// from one somebody else made.
type namedGroupSnapshot struct {
	usable bool
	ids    map[string]struct{}
}

// reconcileOutcome is what reconciling a failed create concluded.
type reconcileOutcome int

const (
	// reconcileNone: no group appeared, so the create genuinely failed.
	reconcileNone reconcileOutcome = iota
	// reconcileAdopted: the create took effect; id is the group to manage.
	reconcileAdopted
	// reconcileMismatch: a same-name group appeared that this configuration did
	// not ask for, or one that could not be verified. Nothing is adopted.
	reconcileMismatch
)

type reconcileResult struct {
	outcome reconcileOutcome
	// id is the adopted group (reconcileAdopted only).
	id string
	// extras are further groups the one request left behind, in ID order.
	extras []string
	// mismatched names the groups that blocked adoption (reconcileMismatch only).
	mismatched []string
}

// isAmbiguousCreateFailure reports whether a failed create leaves it unknown
// whether the group was made.
//
// The Platform device group API creates a large smart group and then fails the
// request: 22,000-member creates ended in `422 A resource by the specified name
// already exists` after about 16s or in a CloudFront `504` after 30s, and in
// every run the group existed afterwards, twice after some 504s. The client sent
// one POST; whatever retries does so behind it. So the response says nothing
// about whether the group exists, and treating it as "not created" orphans a
// live group the next apply then collides with.
//
// A definite client rejection is not ambiguous: a 400, 403 or 409 means the
// request was refused. 422 is in because the name-exists form is exactly the
// retry's signature; a 422 for any other reason reconciles to "nothing appeared"
// and surfaces unchanged. A cancelled context is the operator stopping the run.
func isAmbiguousCreateFailure(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if helpers.IsEdgeBlocked(err) {
		return true
	}
	apiErr, ok := errors.AsType[*jamfplatform.APIResponseError](err)
	if !ok {
		return true
	}
	switch {
	case apiErr.StatusCode >= 500:
		return true
	case apiErr.HasStatus(http.StatusUnprocessableEntity):
		return true
	}
	return false
}

// snapshotNamedGroups lists the groups that already hold name for deviceType. A
// failure to list is not an error: it only turns reconciliation off.
func (r *DeviceGroupResource) snapshotNamedGroups(ctx context.Context, name, deviceType string) namedGroupSnapshot {
	groups, err := r.listNamedGroups(ctx, name, deviceType)
	if err != nil {
		tflog.Debug(ctx, "device group pre-create lookup failed; a failed create will not be reconciled", map[string]any{"error": err.Error()})
		return namedGroupSnapshot{}
	}
	ids := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		ids[g.ID] = struct{}{}
	}
	return namedGroupSnapshot{usable: true, ids: ids}
}

// listNamedGroups returns the groups whose name equals name exactly for the
// device type. The RSQL filter treats `*` as a wildcard and may fold case, so the
// result is narrowed again here rather than trusted.
func (r *DeviceGroupResource) listNamedGroups(ctx context.Context, name, deviceType string) ([]devicegroups.DeviceGroupListReadRepresentationV1, error) {
	filter := filters.Clause("name", "==", name) + " and " + filters.Clause("deviceType", "==", strings.ToUpper(deviceType))
	all, err := r.client.ListDeviceGroups(ctx, nil, filter)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, g := range all {
		if g.Name == name && strings.EqualFold(g.DeviceType, deviceType) {
			out = append(out, g)
		}
	}
	return out, nil
}

func (r *DeviceGroupResource) newNamedGroups(ctx context.Context, name, deviceType string, before namedGroupSnapshot) ([]devicegroups.DeviceGroupListReadRepresentationV1, error) {
	groups, err := r.listNamedGroups(ctx, name, deviceType)
	if err != nil {
		return nil, err
	}
	var fresh []devicegroups.DeviceGroupListReadRepresentationV1
	for _, g := range groups {
		if _, known := before.ids[g.ID]; !known {
			fresh = append(fresh, g)
		}
	}
	return fresh, nil
}

// reconcileCreate decides what a failed create left behind.
//
// It waits for a group that was not there before the request, takes one more
// look to catch a duplicate that lands a moment later, and then requires every
// such group to match what was asked for. All matching: adopt the lowest ID and
// report the rest, since two groups made by one request are identical and the
// choice only has to be deterministic. Any one not matching, or not verifiable:
// adopt nothing, because that group may be someone else's.
//
// The extras are reported, never deleted. The argument that only this request
// could have made them is strong, but the delete is irreversible on a shared
// tenant, so removing them stays the operator's call.
func (r *DeviceGroupResource) reconcileCreate(ctx context.Context, body *devicegroups.DeviceGroupCreateRepresentationV1, before namedGroupSnapshot) reconcileResult {
	var found []devicegroups.DeviceGroupListReadRepresentationV1
	for attempt := 0; attempt < reconcileAttempts && len(found) == 0; attempt++ {
		if attempt > 0 && !sleepCtx(ctx, reconcileInterval) {
			return reconcileResult{outcome: reconcileNone}
		}
		fresh, err := r.newNamedGroups(ctx, body.Name, body.DeviceType, before)
		if err != nil {
			tflog.Debug(ctx, "device group reconcile lookup failed", map[string]any{"attempt": attempt, "error": err.Error()})
			continue
		}
		found = fresh
	}
	if len(found) == 0 {
		return reconcileResult{outcome: reconcileNone}
	}

	if sleepCtx(ctx, reconcileInterval) {
		if again, err := r.newNamedGroups(ctx, body.Name, body.DeviceType, before); err == nil && len(again) >= len(found) {
			found = again
		}
	}

	ids := make([]string, 0, len(found))
	for _, g := range found {
		ids = append(ids, g.ID)
	}
	sort.Strings(ids)

	var mismatched []string
	for _, id := range ids {
		if !r.groupMatchesRequest(ctx, id, body) {
			mismatched = append(mismatched, id)
		}
	}
	if len(mismatched) > 0 {
		return reconcileResult{outcome: reconcileMismatch, mismatched: mismatched}
	}
	return reconcileResult{outcome: reconcileAdopted, id: ids[0], extras: ids[1:]}
}

// groupMatchesRequest reports whether the stored group is the one body asked
// for: device type, group type, criteria for a smart group, members for a static
// one, and a description when one was sent. A group it cannot read is a
// non-match, which is the safe answer.
func (r *DeviceGroupResource) groupMatchesRequest(ctx context.Context, id string, body *devicegroups.DeviceGroupCreateRepresentationV1) bool {
	grp, err := r.client.GetDeviceGroup(ctx, id)
	if err != nil {
		tflog.Debug(ctx, "device group reconcile could not read a candidate", map[string]any{"id": id, "error": err.Error()})
		return false
	}
	if !strings.EqualFold(grp.DeviceType, body.DeviceType) || !strings.EqualFold(grp.GroupType, body.GroupType) {
		return false
	}
	if body.Description != nil && *body.Description != grp.Description {
		return false
	}
	switch {
	case strings.EqualFold(body.GroupType, devicegroups.GroupTypeV1Smart):
		return criteriaEquivalent(body.Criteria, grp.Criteria)
	case strings.EqualFold(body.GroupType, devicegroups.GroupTypeV1Static) && body.Members != nil:
		members, err := r.client.ListDeviceGroupMembers(ctx, id)
		if err != nil {
			return false
		}
		return sameStrings(*body.Members, members)
	}
	return true
}

// criteriaEquivalent compares the criteria sent with the criteria stored, by
// position. Pointer flags that the server omits compare as false, and the join
// type compares case-insensitively; every other field must be identical. A server
// that rewrote a value makes this report a difference, which declines the
// adoption rather than risking a wrong group.
func criteriaEquivalent(sent, stored *[]devicegroups.DeviceGroupCriteriaRepresentationV1) bool {
	var a, b []devicegroups.DeviceGroupCriteriaRepresentationV1
	if sent != nil {
		a = slices.Clone(*sent)
	}
	if stored != nil {
		b = slices.Clone(*stored)
	}
	if len(a) != len(b) {
		return false
	}
	byOrder := func(c []devicegroups.DeviceGroupCriteriaRepresentationV1) {
		sort.SliceStable(c, func(i, j int) bool { return c[i].Order < c[j].Order })
	}
	byOrder(a)
	byOrder(b)
	flag := func(p *bool) bool { return p != nil && *p }
	for i := range a {
		if a[i].Order != b[i].Order ||
			a[i].AttributeName != b[i].AttributeName ||
			a[i].Operator != b[i].Operator ||
			a[i].AttributeValue != b[i].AttributeValue ||
			!strings.EqualFold(a[i].JoinType, b[i].JoinType) ||
			flag(a[i].HasOpeningParenthesis) != flag(b[i].HasOpeningParenthesis) ||
			flag(a[i].HasClosingParenthesis) != flag(b[i].HasClosingParenthesis) {
			return false
		}
	}
	return true
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

// sleepCtx waits d and reports whether the full wait elapsed; false means ctx
// ended first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// adoptedCreateWarning is the warning shown when a failed create is adopted.
//
// It renders the failure with Error(), not helpers.APIErrorDetail, on purpose:
// that helper's advice for a gateway page says the request never reached Jamf
// and nothing changed, which is the one thing this warning exists to contradict.
func adoptedCreateWarning(id string, extras []string, createErr error) (summary, detail string) {
	summary = "Device group was created despite an error response; adopted it"
	detail = fmt.Sprintf("The create request failed with: %s\n\nThe group exists on the tenant, matches this configuration and has been adopted as %s. "+
		"The Platform device group API creates large smart groups and then fails the request, so the failure was not evidence the group was missing.",
		createErr.Error(), id)
	if len(extras) > 0 {
		detail += fmt.Sprintf("\n\nThe same request also left %d identical group(s) with the same name that Terraform does not manage: %s. "+
			"Delete them in Jamf Pro, or with DELETE /device-groups/v1/device-groups/{id}. They were not removed automatically because the delete cannot be undone.",
			len(extras), strings.Join(extras, ", "))
	}
	return summary, detail
}

// mismatchedCreateDetail is appended to the create error when a same-name group
// appeared that this configuration did not ask for.
func mismatchedCreateDetail(ids []string) string {
	return fmt.Sprintf("\n\nAny advice above that nothing changed is not reliable for this endpoint: a group with this name appeared while the request was in flight but does not match this configuration, or could not be read back, so it was not adopted: %s. "+
		"If one of them is the group this request made, import it with `terraform import`; otherwise another writer is using this name.", strings.Join(ids, ", "))
}
