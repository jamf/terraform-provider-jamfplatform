// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_settings

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// days_of_week validates against explicit SDK constants rather than
// AppInstallersDeploymentProcessControlsDaysOfWeekValues(), so an SDK bump cannot
// silently widen what the attribute accepts. This is the tripwire for the
// converse: a value Jamf adds passing unnoticed. Seven days is a closed set in
// practice, which is exactly why a change here would be worth looking at.
func TestDaysOfWeekEnum_HasNotGrown(t *testing.T) {
	want := map[string]bool{
		pro.AppInstallersDeploymentProcessControlsDaysOfWeekMonday:    true,
		pro.AppInstallersDeploymentProcessControlsDaysOfWeekTuesday:   true,
		pro.AppInstallersDeploymentProcessControlsDaysOfWeekWednesday: true,
		pro.AppInstallersDeploymentProcessControlsDaysOfWeekThursday:  true,
		pro.AppInstallersDeploymentProcessControlsDaysOfWeekFriday:    true,
		pro.AppInstallersDeploymentProcessControlsDaysOfWeekSaturday:  true,
		pro.AppInstallersDeploymentProcessControlsDaysOfWeekSunday:    true,
	}
	got := pro.AppInstallersDeploymentProcessControlsDaysOfWeekValues()
	for _, v := range got {
		if !want[v] {
			t.Errorf("AppInstallersDeploymentProcessControlsDaysOfWeek gained value %q: update the days_of_week validator", v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("AppInstallersDeploymentProcessControlsDaysOfWeek has %d values, schema validates %d", len(got), len(want))
	}
}
