// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"strconv"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// The helpers below create, outside Terraform, the object a name collision test
// then plans a colliding resource against. Each stands in for an object someone
// made in the web interface, and removes it when the test ends.

// CreateOutOfBandPlatformDeviceGroup creates a static Platform device group of
// the given device type ("COMPUTER" or "MOBILE") and returns its Platform ID.
func CreateOutOfBandPlatformDeviceGroup(t *testing.T, name, deviceType string) string {
	t.Helper()
	c := devicegroups.New(NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateDeviceGroup(ctx, &devicegroups.DeviceGroupCreateRepresentationV1{
		Name:       name,
		DeviceType: deviceType,
		GroupType:  devicegroups.GroupTypeV1Static,
		Members:    &[]string{},
	})
	if err != nil {
		t.Fatalf("creating Platform device group %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteDeviceGroup(ctx, created.ID) })
	return created.ID
}

// CreateOutOfBandProStaticComputerGroup creates a static computer group through
// the Jamf Pro API and returns its ID.
func CreateOutOfBandProStaticComputerGroup(t *testing.T, name string) string {
	t.Helper()
	c := pro.New(NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateStaticComputerGroupV3(ctx, &pro.StaticComputerGroupAssignment{
		Name:        name,
		Assignments: &[]string{},
	}, true)
	if err != nil {
		t.Fatalf("creating static computer group %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteStaticComputerGroupV3(ctx, created.ID) })
	return created.ID
}

// CreateOutOfBandClassicUserGroup creates a static user group through the
// Classic API and returns its ID.
func CreateOutOfBandClassicUserGroup(t *testing.T, name string) string {
	t.Helper()
	c := proclassic.New(NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateUserGroupByID(ctx, "0", &proclassic.UserGroup{Name: &name, IsSmart: new(false)})
	if err != nil || created.ID == nil {
		t.Fatalf("creating user group %q outside Terraform: %v", name, err)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeleteUserGroupByID(ctx, id) })
	return id
}

// CreateOutOfBandClassicPolicy creates a disabled policy through the Classic API
// and returns its ID.
func CreateOutOfBandClassicPolicy(t *testing.T, name string) string {
	t.Helper()
	c := proclassic.New(NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreatePolicyByID(ctx, "0", &proclassic.PolicyPost{
		General: &proclassic.PolicyPostGeneral{Name: &name, Enabled: new(false)},
	})
	if err != nil || created.ID == nil {
		t.Fatalf("creating policy %q outside Terraform: %v", name, err)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeletePolicyByID(ctx, id) })
	return id
}
