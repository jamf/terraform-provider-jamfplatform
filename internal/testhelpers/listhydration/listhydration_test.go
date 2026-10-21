// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package listhydration

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type wireIcon struct {
	ID *int
}

type wireBlock struct {
	Name  *string
	Icon  *wireIcon
	Cats  *[]wireIcon
	Flags []bool
}

type modelBlock struct {
	Name  types.String
	Icon  *wireIcon
	Cats  []wireIcon
	Set   types.Set
	Count types.Int64
}

func TestFill_PopulatesEveryField(t *testing.T) {
	var w wireBlock
	Fill(&w)
	if w.Name == nil || w.Icon == nil || w.Icon.ID == nil || w.Cats == nil || len(*w.Cats) != 1 || (*w.Cats)[0].ID == nil || len(w.Flags) != 1 {
		t.Fatalf("Fill left a field empty: %+v", w)
	}
}

func TestNilBlocks_ReportsNullBlocksNotScalars(t *testing.T) {
	got := NilBlocks(&modelBlock{})
	want := []string{"Cats", "Icon", "Set"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NilBlocks = %v, want %v", got, want)
	}
}

func TestNilBlocks_HonoursAllowedAndPopulated(t *testing.T) {
	m := &modelBlock{Icon: &wireIcon{}, Cats: []wireIcon{}, Set: types.SetNull(types.StringType)}
	if got := NilBlocks(m, "Set"); len(got) != 0 {
		t.Errorf("NilBlocks = %v, want none", got)
	}
}
