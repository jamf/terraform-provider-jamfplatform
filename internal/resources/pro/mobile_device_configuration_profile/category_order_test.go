// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"slices"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func sortedWireCategories() *proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategories {
	item := func(id int, name string) proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem {
		return proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem{ID: &id, Name: &name}
	}
	return &proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategories{
		Category: &[]proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem{item(1, "a"), item(2, "b"), item(3, "c")},
	}
}

func categoryIDsAndNames(items []SelfServiceCategoryItem) []string {
	var out []string
	for _, c := range items {
		out = append(out, c.ID.ValueString()+"="+c.Name.ValueString())
	}
	return out
}

// Jamf Pro stores the categories sorted by id (wire-probed 2026-11-11), so the
// read must be put back into the planned or prior order.
func TestFlattenSelfServiceCategories_KeepsThePlannedOrder(t *testing.T) {
	t.Parallel()
	order := []SelfServiceCategoryItem{{ID: types.StringValue("3")}, {ID: types.StringValue("1")}}

	if got, want := categoryIDsAndNames(flattenSelfServiceCategories(sortedWireCategories(), order)), []string{"3=c", "1=a", "2=b"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := categoryIDsAndNames(flattenSelfServiceCategories(sortedWireCategories(), nil)), []string{"1=a", "2=b", "3=c"}; !slices.Equal(got, want) {
		t.Errorf("with no order the wire order must stand: got %v, want %v", got, want)
	}
}

func TestFlattenSelfService_CategoriesInPlannedOrder(t *testing.T) {
	t.Parallel()
	t.Run("managed: planned descending order wins over the sorted wire", func(t *testing.T) {
		t.Parallel()
		state := &SelfServiceModel{Categories: []SelfServiceCategoryItem{
			{ID: types.StringValue("3")}, {ID: types.StringValue("2")}, {ID: types.StringValue("1")},
		}}
		flattenSelfService(&proclassic.MobileDeviceConfigurationProfileSelfService{SelfServiceCategories: sortedWireCategories()}, state, false)
		if got, want := categoryIDsAndNames(state.Categories), []string{"3=c", "2=b", "1=a"}; !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("import: no prior, wire order", func(t *testing.T) {
		t.Parallel()
		state := &SelfServiceModel{}
		flattenSelfService(&proclassic.MobileDeviceConfigurationProfileSelfService{SelfServiceCategories: sortedWireCategories()}, state, true)
		if got, want := categoryIDsAndNames(state.Categories), []string{"1=a", "2=b", "3=c"}; !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

// TestFlattenSelfServiceCategories_UnknownIDKeepsItsPosition pins issue #568:
// a planned category whose id is unknown (created in the same apply) takes the
// wire category no other element claimed, at its own position, and carries
// that category's name. Sorting by key pushed it to the end, so planned
// [new, 5] over wire [3, 5] read back [5, 3].
func TestFlattenSelfServiceCategories_UnknownIDKeepsItsPosition(t *testing.T) {
	t.Parallel()
	wireOf := func(ids ...int) *proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategories {
		items := make([]proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem, 0, len(ids))
		for _, id := range ids {
			name := "n" + strconv.Itoa(id)
			items = append(items, proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem{ID: &id, Name: &name})
		}
		return &proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategories{Category: &items}
	}
	unknown := SelfServiceCategoryItem{ID: types.StringUnknown(), Name: types.StringUnknown()}
	known := func(id string) SelfServiceCategoryItem {
		return SelfServiceCategoryItem{ID: types.StringValue(id), Name: types.StringUnknown()}
	}
	tests := []struct {
		name  string
		wire  []int
		order []SelfServiceCategoryItem
		want  []string
	}{
		{"unknown first", []int{3, 5}, []SelfServiceCategoryItem{unknown, known("5")}, []string{"3=n3", "5=n5"}},
		{"unknown last", []int{3, 5}, []SelfServiceCategoryItem{known("5"), unknown}, []string{"5=n5", "3=n3"}},
		{"two unknowns take wire order", []int{3, 4}, []SelfServiceCategoryItem{unknown, unknown}, []string{"3=n3", "4=n4"}},
		{"null id is keyless too", []int{3, 5}, []SelfServiceCategoryItem{{ID: types.StringNull()}, known("5")}, []string{"3=n3", "5=n5"}},
		{"unclaimed wire entry trails", []int{3, 5, 7}, []SelfServiceCategoryItem{known("7"), unknown}, []string{"7=n7", "3=n3", "5=n5"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := categoryIDsAndNames(flattenSelfServiceCategories(wireOf(tt.wire...), tt.order)); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
