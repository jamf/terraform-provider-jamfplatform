// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"slices"
	"testing"
)

func TestOrderByKey(t *testing.T) {
	id := func(s string) string { return s }
	cases := map[string]struct {
		items, order, want []string
	}{
		"server sorted, planned descending": {items: []string{"1", "2", "3"}, order: []string{"3", "2", "1"}, want: []string{"3", "2", "1"}},
		"no prior keeps server order":       {items: []string{"1", "3", "2"}, order: nil, want: []string{"1", "3", "2"}},
		"unnamed items trail in wire order": {items: []string{"1", "4", "2", "5"}, order: []string{"2", "1"}, want: []string{"2", "1", "4", "5"}},
		"order names an id the server lost": {items: []string{"1", "2"}, order: []string{"9", "2", "1"}, want: []string{"2", "1"}},
		"repeated key in order uses first":  {items: []string{"1", "2"}, order: []string{"2", "1", "2"}, want: []string{"2", "1"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := OrderByKey(slices.Clone(tc.items), tc.order, id)
			if !slices.Equal(got, tc.want) {
				t.Errorf("OrderByKey(%v, %v) = %v, want %v", tc.items, tc.order, got, tc.want)
			}
		})
	}
}
