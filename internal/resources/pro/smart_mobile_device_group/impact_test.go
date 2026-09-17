// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

func TestCriterionListsDiffer(t *testing.T) {
	base := []criteria.CriterionModel{criterion("Model", "like", "iPad")}

	if criterionListsDiffer(base, base) {
		t.Error("identical criteria must not read as changed")
	}
	if !criterionListsDiffer(base, nil) {
		t.Error("adding the first criterion must read as changed")
	}
	if !criterionListsDiffer(base, []criteria.CriterionModel{criterion("Model", "like", "iPhone")}) {
		t.Error("an edited value must read as changed")
	}
	if !criterionListsDiffer(base, []criteria.CriterionModel{criterion("Model", "is", "iPad")}) {
		t.Error("an edited comparison must read as changed")
	}
	if !criterionListsDiffer(base, []criteria.CriterionModel{criterion("Model Identifier", "like", "iPad")}) {
		t.Error("an edited inventory attribute must read as changed")
	}
}

// TestCriterionListsDiffer_DetectsAReorder matters because the join between two
// criteria is positional: swapping them changes which value the `and_or` applies
// to and so changes membership.
func TestCriterionListsDiffer_DetectsAReorder(t *testing.T) {
	a := []criteria.CriterionModel{
		criterion("Model", "like", "iPad"),
		criterion("Supervised", "is", "true"),
	}
	b := []criteria.CriterionModel{
		criterion("Supervised", "is", "true"),
		criterion("Model", "like", "iPad"),
	}
	if !criterionListsDiffer(a, b) {
		t.Error("a reorder must read as a membership change")
	}
}

// TestCriterionListsDiffer_IgnoresPriority keeps a renumbering out of the alert.
// The priority is the criterion's own position, which the builders emit from the
// list index, so a differing stored value says nothing about membership.
func TestCriterionListsDiffer_IgnoresPriority(t *testing.T) {
	a := []criteria.CriterionModel{criterion("Model", "like", "iPad")}
	b := []criteria.CriterionModel{criterion("Model", "like", "iPad")}
	a[0].Priority = types.Int64Value(0)
	b[0].Priority = types.Int64Value(3)
	if criterionListsDiffer(a, b) {
		t.Error("a differing priority alone must not read as a membership change")
	}
}

// TestMembershipAlert_ReportsNothingForARename is the shape the alert exists to
// avoid: Jamf Pro alerts on a criteria edit, not on an edit that leaves
// membership alone.
func TestMembershipAlert_ReportsNothingForARename(t *testing.T) {
	unchanged := []criteria.CriterionModel{criterion("Model", "like", "iPad")}
	got := impact.ReportMembership(t.Context(), impact.MembershipRequest{
		Cache:  enabledCache(t),
		Label:  progroups.Label(progroups.DeviceTypeMobile, progroups.GroupKindSmart),
		Action: impact.ActionUpdate,
		Membership: impact.Membership{
			Noun:         progroups.MemberNoun(progroups.DeviceTypeMobile),
			Undetermined: impact.CriteriaUndetermined,
			Changed:      criterionListsDiffer(unchanged, unchanged),
		},
	})
	if len(got) != 0 {
		t.Errorf("a rename must raise no alert, got %v", got)
	}
}

// TestMembershipAlert_ReportsACriteriaEditAsUndetermined pins the one thing this
// construct can say about a smart group: who ends up in it is Jamf Pro's to
// decide after the write, so the alert reports the effect without a count.
func TestMembershipAlert_ReportsACriteriaEditAsUndetermined(t *testing.T) {
	got := impact.ReportMembership(t.Context(), impact.MembershipRequest{
		Cache:  enabledCache(t),
		Label:  progroups.Label(progroups.DeviceTypeMobile, progroups.GroupKindSmart),
		Action: impact.ActionUpdate,
		Membership: impact.Membership{
			Noun:         progroups.MemberNoun(progroups.DeviceTypeMobile),
			Undetermined: impact.CriteriaUndetermined,
			Changed: criterionListsDiffer(
				[]criteria.CriterionModel{criterion("Model", "like", "iPad")},
				[]criteria.CriterionModel{criterion("Model", "like", "iPhone")},
			),
		},
	})
	if len(got) != 1 {
		t.Fatalf("expected one advisory, got %v", got)
	}
	if got[0].Severity().String() != "Warning" {
		t.Errorf("an impact alert must be advisory, got %s", got[0].Severity())
	}
	if !strings.Contains(got[0].Detail(), "not known during plan") {
		t.Errorf("the alert must say the resulting membership is not known, got:\n%s", got[0].Detail())
	}
	if !strings.Contains(got[0].Summary(), "smart mobile device group") {
		t.Errorf("the alert must name the object the way the admin UI does, got: %s", got[0].Summary())
	}
}

// TestReportMembershipImpact_SilentWithoutProviderData covers the unit-test and
// early-Configure path, where the resource has no provider data and so no cache.
func TestReportMembershipImpact_SilentWithoutProviderData(t *testing.T) {
	r := &SmartMobileDeviceGroupResource{}
	if r.pd != nil {
		t.Fatal("a fresh resource must carry no provider data")
	}
	// A nil cache disables reporting, which is what keeps every resource in the
	// family free of an enabled-flag check.
	var disabled *impact.Cache
	if disabled.Enabled() {
		t.Error("a nil cache must report as disabled")
	}
}

// enabledCache returns a cache with alerts turned on. A membership alert reads
// nothing, so the source behind it never answers a call; it exists only because
// a cache with no source is the provider's "alerts are off" signal.
func enabledCache(t *testing.T) *impact.Cache {
	t.Helper()
	return impact.NewCache(unreadSource{t: t})
}

// unreadSource fails the test if a membership alert reaches for a read. Nothing
// in the smart-group path should: the group's membership is not knowable at plan
// time, so there is nothing to look up.
type unreadSource struct{ t *testing.T }

func (s unreadSource) Groups(context.Context) ([]impact.Group, error) {
	s.t.Error("a membership alert must not read the tenant group list")
	return nil, nil
}

func (s unreadSource) Totals(context.Context) (impact.Totals, error) {
	s.t.Error("a membership alert must not read the device totals")
	return impact.Totals{}, nil
}

func (s unreadSource) Members(context.Context, string) ([]string, error) {
	s.t.Error("a membership alert must not read group members")
	return nil, nil
}

func (s unreadSource) ComputerManagementIDs(context.Context, string) ([]string, error) {
	s.t.Error("a membership alert must not read the computer inventory")
	return nil, nil
}

func (s unreadSource) MobileManagementIDs(context.Context, string) ([]string, error) {
	s.t.Error("a membership alert must not read the mobile device inventory")
	return nil, nil
}

func (s unreadSource) PlaceNames(context.Context) (impact.Places, error) {
	s.t.Error("a membership alert must not read building or department names")
	return impact.Places{}, nil
}
