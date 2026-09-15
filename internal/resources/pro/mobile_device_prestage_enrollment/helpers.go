// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// injectVersionLocks copies the THREE optimistic-lock counters (root +
// locationInformation + purchasingInformation) from a fresh GET into the PUT
// body. Mobile prestages have no accountSettings layer, so there is no fourth
// lock here (spike §5, §7). `names` and `skipSetupItems` carry no versionLock.
// Per live wire-probe a nested versionLock mismatch triggers a 409
// OPTIMISTIC_LOCK_FAILED regardless of the root lock; all three must echo back
// verbatim.
func injectVersionLocks(put *pro.PutMobileDevicePrestageV3, got *pro.GetMobileDevicePrestageV3) {
	rootLock := got.VersionLock
	put.VersionLock = &rootLock

	put.LocationInformation.ID = ""
	put.LocationInformation.VersionLock = 0
	if got.LocationInformation != nil {
		put.LocationInformation.ID = got.LocationInformation.ID
		put.LocationInformation.VersionLock = got.LocationInformation.VersionLock
	}

	put.PurchasingInformation.ID = ""
	put.PurchasingInformation.VersionLock = 0
	if got.PurchasingInformation != nil {
		put.PurchasingInformation.ID = got.PurchasingInformation.ID
		put.PurchasingInformation.VersionLock = got.PurchasingInformation.VersionLock
	}
}

// isPutSerializerBug returns true when err is the empty-errors[] 500 returned
// by /v3/mobile-device-prestages PUT (Jamf server-side bug — response
// serializer crashes downstream of a successful write, §F4). When this matches
// the caller must GET the record and diff every plan field to decide between
// "500-with-commit" and "500-with-silent-rollback" per spike §F4 / §F4b.
//
// Detection signature is narrow: HTTP status 500 + the exact body
// `{"httpStatus":500,"errors":[]}`. Any 500 with populated errors propagates
// as a real failure.
func isPutSerializerBug(err error) bool {
	if err == nil {
		return false
	}
	var apiErr interface {
		HasStatus(int) bool
	}
	if !errors.As(err, &apiErr) {
		return false
	}
	if !apiErr.HasStatus(500) {
		return false
	}
	msg := err.Error()
	// The SDK formats the upstream body verbatim; the Jamf bug returns an
	// empty errors array. Be strict — match exact substring so a real 500
	// (e.g. internal infrastructure failure) with populated errors does not
	// silently trip the fallback path.
	return strings.Contains(msg, `"errors" : [ ]`) || strings.Contains(msg, `"errors":[]`)
}

// putWorkaroundWarning is the single source of truth for the user-facing
// one-shot warning text logged when the 500-with-commit code path fires.
const putWorkaroundWarning = "Jamf Pro mobile-device-prestage PUT returned HTTP 500 with an empty errors[] body; the underlying write committed. " +
	"This is a known Jamf Pro defect; see the tracking issue. " +
	"The provider verified the write via GET and is treating it as successful."

// fmtUnchangedFields formats a list of plan-vs-GET mismatched field paths for
// inclusion in the silent-rollback diagnostic.
func fmtUnchangedFields(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return fmt.Sprintf("Fields that did not round-trip: %s", strings.Join(paths, ", "))
}
