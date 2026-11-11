// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"cmp"
	"slices"
)

// OrderByKey sorts items, in place and stably, into the order their keys
// first appear in order, and returns items. An item whose key order does not
// name keeps its relative position after every named one. With an empty order
// (no prior state: import, config generation) the items keep the order they
// arrived in.
//
// It exists for a list Jamf Pro stores in an order of its own choosing, where
// the written order carries meaning only to the configuration: the classic
// configuration profiles return Self Service categories sorted by id, so a
// list written in any other order fails the apply with "Provider produced
// inconsistent result after apply" unless the read is put back into the
// planned (create, update) or prior (read) order.
func OrderByKey[T any](items, order []T, key func(T) string) []T {
	if len(order) == 0 {
		return items
	}
	rank := make(map[string]int, len(order))
	for i, o := range order {
		if _, seen := rank[key(o)]; !seen {
			rank[key(o)] = i
		}
	}
	position := func(it T) int {
		if r, ok := rank[key(it)]; ok {
			return r
		}
		return len(order)
	}
	slices.SortStableFunc(items, func(a, b T) int {
		return cmp.Compare(position(a), position(b))
	})
	return items
}
