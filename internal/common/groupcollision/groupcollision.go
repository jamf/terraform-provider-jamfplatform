// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package groupcollision builds the name uniqueness namespaces for Jamf Pro
// device groups, shared by the four Jamf Pro group constructs and
// jamfplatform_device_group.
//
// Those five constructs write to three different APIs but all draw on one pool
// of names per device type, so a name taken through any of them, or through the
// Classic API, refuses the same name through every other. One listing serves
// the whole pool: /pro/v2/groups returns every computer and mobile device group,
// smart and static, whichever API created it, with both the Jamf Pro and the
// Platform identifier. Wire-probed 2026-10-25 against Jamf Pro 11.33.1 (EU,
// environment scope).
//
// The collation is the rule of the JSON APIs these constructs write through,
// which is not the Classic API's. Computer groups compare case-insensitively and
// treat a trailing space, a leading space and an accent as part of the name.
// Mobile device groups also fold trailing spaces and accents. A Classic write
// trims its XML text first and compares accents exactly for both, so reusing the
// Classic rule here would report collisions the JSON APIs accept.
package groupcollision

import (
	"context"
	"fmt"
	"sync"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

// DeviceGroupResourceType is the Terraform type of the Platform device group,
// which imports a group by its Platform identifier rather than its Jamf Pro one.
const DeviceGroupResourceType = "jamfplatform_device_group"

const (
	smartComputerGroupType  = "jamfplatform_pro_smart_computer_group"
	staticComputerGroupType = "jamfplatform_pro_static_computer_group"
	smartMobileGroupType    = "jamfplatform_pro_smart_mobile_device_group"
	staticMobileGroupType   = "jamfplatform_pro_static_mobile_device_group"
)

type lister func(ctx context.Context, sort []string, filter string) ([]pro.GroupDtoV1, error)

// ComputerNamespace is the name pool shared by smart and static computer groups
// and Platform device groups of type COMPUTER.
func ComputerNamespace(client *pro.Client) collision.Namespace {
	return computerNamespace(client.ListGroupsV2)
}

// MobileNamespace is the name pool shared by smart and static mobile device
// groups and Platform device groups of type MOBILE.
func MobileNamespace(client *pro.Client) collision.Namespace {
	return mobileNamespace(client.ListGroupsV2)
}

func computerNamespace(list lister) collision.Namespace {
	return namespace(list, spec{
		id:         "pro.computer-groups",
		groupType:  pro.GroupDtoV1GroupTypeComputer,
		noun:       "computer group",
		smartType:  smartComputerGroupType,
		staticType: staticComputerGroupType,
		collation:  collision.CollationCaseOnly,
	})
}

func mobileNamespace(list lister) collision.Namespace {
	return namespace(list, spec{
		id:         "pro.mobile-device-groups",
		groupType:  pro.GroupDtoV1GroupTypeMobile,
		noun:       "mobile device group",
		smartType:  smartMobileGroupType,
		staticType: staticMobileGroupType,
		collation:  collision.CollationFolded,
	})
}

type spec struct {
	id         string
	groupType  string
	noun       string
	smartType  string
	staticType string
	collation  collision.Collation
}

func namespace(list lister, s spec) collision.Namespace {
	var (
		once sync.Once
		rows []pro.GroupDtoV1
		err  error
	)
	fetch := func(ctx context.Context) ([]pro.GroupDtoV1, error) {
		once.Do(func() {
			rows, err = list(ctx, nil, fmt.Sprintf("groupType==%q", s.groupType))
		})
		return rows, err
	}
	source := func(smart bool, resourceType string) collision.Source {
		kind := "static"
		if smart {
			kind = "smart"
		}
		return collision.Source{
			Noun:         kind + " " + s.noun,
			ResourceType: resourceType,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				all, err := fetch(ctx)
				if err != nil {
					return nil, err
				}
				return entries(all, smart), nil
			},
		}
	}
	return collision.Namespace{
		ID:        s.id,
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: s.collation,
		Sources:   []collision.Source{source(true, s.smartType), source(false, s.staticType)},
	}
}

func entries(rows []pro.GroupDtoV1, smart bool) []collision.Entry {
	var out []collision.Entry
	for _, r := range rows {
		if r.Smart != smart || r.GroupJamfProID == "" {
			continue
		}
		e := collision.Entry{ID: r.GroupJamfProID, Key: r.GroupName}
		if r.GroupPlatformID != "" {
			e.AltIDs = map[string]string{DeviceGroupResourceType: r.GroupPlatformID}
		}
		out = append(out, e)
	}
	return out
}
