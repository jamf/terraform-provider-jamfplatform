// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package groupcollision

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

func rows() []pro.GroupDtoV1 {
	return []pro.GroupDtoV1{
		{GroupName: "Lab Macs", GroupJamfProID: "5", GroupPlatformID: "uuid-5", GroupType: pro.GroupDtoV1GroupTypeComputer, Smart: true},
		{GroupName: "Kiosks", GroupJamfProID: "6", GroupPlatformID: "uuid-6", GroupType: pro.GroupDtoV1GroupTypeComputer},
		{GroupName: "Café", GroupJamfProID: "7", GroupType: pro.GroupDtoV1GroupTypeMobile},
	}
}

func checker() *collision.Checker {
	return collision.New(collision.Config{Mode: collision.ModeError})
}

func request(ns collision.Namespace, resourceType, key, selfID string) collision.Request {
	return collision.Request{
		Namespace:    ns,
		ResourceType: resourceType,
		Path:         path.Root("name"),
		Keys:         []string{key},
		SelfID:       selfID,
	}
}

func TestComputerNamespace_SmartAndStaticShareOnePool(t *testing.T) {
	var calls atomic.Int32
	ns := computerNamespace(func(context.Context, []string, string) ([]pro.GroupDtoV1, error) {
		calls.Add(1)
		return rows(), nil
	})
	c := checker()

	d := c.Check(context.Background(), request(ns, staticComputerGroupType, "LAB MACS", ""))
	if !d.HasError() || !strings.Contains(d[0].Detail(), "smart computer group") {
		t.Fatalf("a static group must collide with a smart group of the same name, got %v", d)
	}
	d = c.Check(context.Background(), request(ns, smartComputerGroupType, "kiosks", ""))
	if !d.HasError() || !strings.Contains(d[0].Detail(), "static computer group") {
		t.Fatalf("a smart group must collide with a static group of the same name, got %v", d)
	}
	if calls.Load() != 1 {
		t.Fatalf("one listing must serve the whole pool, got %d", calls.Load())
	}
}

func TestComputerNamespace_RequestsOnlyItsDeviceType(t *testing.T) {
	var filter string
	ns := computerNamespace(func(_ context.Context, _ []string, f string) ([]pro.GroupDtoV1, error) {
		filter = f
		return nil, nil
	})
	if _, err := ns.Sources[0].List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if filter != `groupType=="COMPUTER"` {
		t.Fatalf("filter = %q", filter)
	}
}

func TestComputerNamespace_ComparesCaseOnly(t *testing.T) {
	ns := computerNamespace(func(context.Context, []string, string) ([]pro.GroupDtoV1, error) {
		return []pro.GroupDtoV1{{GroupName: "Cafe", GroupJamfProID: "9", GroupType: pro.GroupDtoV1GroupTypeComputer}}, nil
	})
	c := checker()
	for _, key := range []string{"Café", "Cafe ", " Cafe"} {
		if d := c.Check(context.Background(), request(ns, staticComputerGroupType, key, "")); len(d) != 0 {
			t.Errorf("%q is a distinct computer group name on the JSON APIs, got %v", key, d)
		}
	}
	if d := c.Check(context.Background(), request(ns, staticComputerGroupType, "CAFE", "")); !d.HasError() {
		t.Error("a case difference alone must collide")
	}
}

func TestMobileNamespace_FoldsCaseSpacesAndAccents(t *testing.T) {
	ns := mobileNamespace(func(context.Context, []string, string) ([]pro.GroupDtoV1, error) { return rows(), nil })
	c := checker()
	for _, key := range []string{"cafe", "CAFÉ", "Café "} {
		if d := c.Check(context.Background(), request(ns, staticMobileGroupType, key, "")); !d.HasError() {
			t.Errorf("%q must collide on the mobile pool", key)
		}
	}
	if d := c.Check(context.Background(), request(ns, staticMobileGroupType, " Café", "")); len(d) != 0 {
		t.Errorf("a leading space is a distinct name, got %v", d)
	}
}

func TestComputerNamespace_PlatformGroupImportsByPlatformID(t *testing.T) {
	ns := computerNamespace(func(context.Context, []string, string) ([]pro.GroupDtoV1, error) { return rows(), nil })
	d := checker().Check(context.Background(), request(ns, DeviceGroupResourceType, "lab macs", ""))
	if !d.HasError() {
		t.Fatal("want a collision")
	}
	detail := d[0].Detail()
	if !strings.Contains(detail, `id = "uuid-5"`) || strings.Contains(detail, `"5"`) {
		t.Errorf("a device group must be imported by its Platform identifier:\n%s", detail)
	}
}

func TestComputerNamespace_SelfExcludedUnderEitherIdentifier(t *testing.T) {
	ns := computerNamespace(func(context.Context, []string, string) ([]pro.GroupDtoV1, error) { return rows(), nil })
	c := checker()
	if d := c.Check(context.Background(), request(ns, staticComputerGroupType, "kiosks", "6")); len(d) != 0 {
		t.Errorf("a Jamf Pro group must not collide with itself, got %v", d)
	}
	if d := c.Check(context.Background(), request(ns, DeviceGroupResourceType, "kiosks", "uuid-6")); len(d) != 0 {
		t.Errorf("a device group must not collide with itself, got %v", d)
	}
}

func TestNamespace_SkipsRowsWithoutAJamfProID(t *testing.T) {
	got := entries([]pro.GroupDtoV1{{GroupName: "x", GroupPlatformID: "u"}}, false)
	if len(got) != 0 {
		t.Fatalf("a row with no Jamf Pro identifier cannot be imported by a Jamf Pro resource, got %v", got)
	}
}
