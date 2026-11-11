// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"slices"
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
