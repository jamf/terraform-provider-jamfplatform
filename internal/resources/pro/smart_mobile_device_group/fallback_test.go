// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// groupEndpointError builds the shape the group endpoints return: a status plus
// decoded details carrying a code, a field and a description. The refusals this
// gate turns on are distinguished by the description, so that is what each case
// varies.
func groupEndpointError(status int, code, field, description string) error {
	return &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     http.MethodPost,
		URL:        "https://example.invalid/pro/v2/mobile-device-groups/smart-groups",
		Errors: []jamfplatform.ErrorDetail{{
			Code:        code,
			Field:       field,
			Description: description,
		}},
	}
}

// TestFallbackApplies_OnlyOnARefusedCriterionOrOperator pins both halves of the
// gate, and the second half is the one worth having.
//
// A refusal of a criterion or an operator takes the fallback, because the older
// group interface stores those combinations. Everything else must not: a
// duplicate name, a site the integration cannot reach and a malformed andOr are
// all refused by both interfaces, so a second write would spend a request to
// reach the same answer with a worse message, and the operator would be told the
// provider worked around a Jamf Pro defect that had nothing to do with their
// configuration.
func TestFallbackApplies_OnlyOnARefusedCriterionOrOperator(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"a refused criterion": {
			err:  groupEndpointError(http.StatusBadRequest, "INVALID_FIELD", "criteria", "The criterion Patch Reporting: 1Password is not valid"),
			want: true,
		},
		"a refused operator": {
			err:  groupEndpointError(http.StatusBadRequest, "INVALID_FIELD", "criteria", "The operator has is not valid for extension attribute Asset Tag"),
			want: true,
		},
		"an unrelated invalid field": {
			err: groupEndpointError(http.StatusBadRequest, "INVALID_FIELD", "criteria", "The andOr value AND_THEN is not valid"),
		},
		"a duplicate group name": {
			err: groupEndpointError(http.StatusUnprocessableEntity, "DUPLICATE_FIELD", "name", "Group named Supervised iPads already exists"),
		},
		"a refused site": {
			err: groupEndpointError(http.StatusForbidden, "INVALID_PRIVILEGE", "siteId", "Access denied: insufficient privileges"),
		},
		"a gateway failure": {
			err: groupEndpointError(http.StatusInternalServerError, "", "", ""),
		},
		"a plain error": {
			err: errors.New("dial tcp: connection refused"),
		},
		"no error at all": {},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := fallbackApplies(tc.err); got != tc.want {
				t.Errorf("fallbackApplies = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestFallbackApplies_DoesNotTriggerOnTheCriterionName is the guard against the
// tempting shortcut.
//
// Matching "Patch Reporting:" would cover one of the three known refusals and
// nothing that comes after them, and it would fire on a criterion Jamf Pro
// accepted whose write failed for some other reason. The gate reads the refusal,
// so a patch reporting criterion refused for a different cause takes the ordinary
// error path.
func TestFallbackApplies_DoesNotTriggerOnTheCriterionName(t *testing.T) {
	err := groupEndpointError(http.StatusUnprocessableEntity, "DUPLICATE_FIELD", "name", "Group named Patch Reporting: 1Password already exists")
	if fallbackApplies(err) {
		t.Error("the gate must read the refusal, not the criterion's name")
	}
}
