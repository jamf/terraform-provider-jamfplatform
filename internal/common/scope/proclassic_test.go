// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package scope

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestFlattenIDNameSet(t *testing.T) {
	ctx := context.Background()
	if got := FlattenIDNameSet(ctx, nil); got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("nil items should be empty set, got %v", got)
	}
	items := &[]proclassic.IDName{{ID: new(1), Name: new("a")}, {ID: new(2), Name: new("b")}}
	got := FlattenIDNameSet(ctx, items)
	if got.IsNull() || len(got.Elements()) != 2 {
		t.Errorf("expected 2-element set, got %v", got)
	}
}

func TestFlattenNameSet(t *testing.T) {
	ctx := context.Background()
	if got := FlattenNameSet(ctx, nil); got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("nil items should be empty set, got %v", got)
	}
	items := &[]proclassic.IDName{{ID: new(1), Name: new("alpha")}}
	got := FlattenNameSet(ctx, items)
	if got.IsNull() || len(got.Elements()) != 1 {
		t.Errorf("expected 1-element set, got %v", got)
	}
}

func TestFlattenSiteObject(t *testing.T) {
	id, name := FlattenSiteObject(nil)
	if id != nil || name != nil {
		t.Errorf("nil site should yield (nil,nil)")
	}
	id, name = FlattenSiteObject(&proclassic.SiteObject{ID: new(7), Name: new("HQ")})
	if id == nil || *id != "7" || name == nil || *name != "HQ" {
		t.Errorf("got (%v,%v), want (\"7\",\"HQ\")", id, name)
	}
	// "NONE" sentinel (id -1): the classic GET nondeterministically echoes or omits
	// the name, so it must be nilled regardless of what the server returned.
	id, name = FlattenSiteObject(&proclassic.SiteObject{ID: new(-1), Name: new("NONE")})
	if id == nil || *id != "-1" || name != nil {
		t.Errorf("got (%v,%v), want (\"-1\",nil) for the NONE sentinel", id, name)
	}
}

func TestBuildSiteObject(t *testing.T) {
	for _, in := range []types.String{types.StringNull(), types.StringUnknown(), types.StringValue(""), types.StringValue("nope")} {
		if got := BuildSiteObject(in); got != nil {
			t.Errorf("BuildSiteObject(%v) = %v, want nil", in, got)
		}
	}
	got := BuildSiteObject(types.StringValue("12"))
	if got == nil || got.ID == nil || *got.ID != 12 {
		t.Errorf("BuildSiteObject(\"12\") = %v, want ID 12", got)
	}
}
