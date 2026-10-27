// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package collision

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// CreateConflictHint explains a duplicate refusal on a create for a resource type checked
// against a namespace Jamf enforces, as a paragraph that starts with a blank
// line so a caller appends it to an error detail with a plain +. It returns ""
// for any other error, a nil Checker, or a type with no enforced rule.
//
// The plan-time check lists the tenant once, so two new objects declared with
// one key both pass it: neither exists yet, and a resource cannot see a
// sibling's plan. The second create then reaches Jamf and is refused. The hint
// names that cause, which the bare 409 does not. Callers append it to the API
// error detail. The attributes it names are the ones the resource's plan-time
// rules compare, recorded when the resource was planned.
func (c *Checker) CreateConflictHint(err error, resourceType string) string {
	if c == nil || err == nil {
		return ""
	}
	if !isDuplicateRefusal(err) {
		return ""
	}
	var labels []string
	product := ""
	for _, ns := range c.rulesFor(resourceType) {
		if !ns.Enforced {
			continue
		}
		labels = append(labels, ns.keyLabel())
		product = ns.Product
	}
	if len(labels) == 0 {
		return ""
	}
	which := joinAnd(labels)
	return fmt.Sprintf("\n\nIf this is a duplicate: %s does not allow two objects with the same %s, and the plan found none. Another resource in this configuration probably declares the same %s and was created first in this apply. Give each a distinct value. If no resource does, something else created it after the plan.",
		product, which, which)
}

// duplicateMarkers are the phrases an API uses, in a 400 or 422 error detail, to
// refuse a value another object already holds: Jamf Pro v1 answers
// "[DUPLICATE_FIELD] name: duplicate name" with a 400, where Classic and most
// Platform services answer 409.
var duplicateMarkers = []string{"duplicate", "already exist", "already in use", "must be unique"}

// isDuplicateRefusal reports whether err is an API refusal of a duplicate: any
// 409, or a 400 or 422 whose structured error details say so. The match is
// deliberately loose, since the hint it gates reads as a possibility and not a
// diagnosis.
func isDuplicateRefusal(err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusConflict:
		return true
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		for _, detail := range apiErr.Details() {
			text := strings.ToLower(detail.Code + " " + detail.Description)
			for _, marker := range duplicateMarkers {
				if strings.Contains(text, marker) {
					return true
				}
			}
		}
	}
	return false
}
