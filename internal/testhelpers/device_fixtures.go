// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// Static group membership needs devices, and borrowing the tenant's own
// inventory does not work.
//
// A test that lists existing devices and skips when it finds none passes
// vacuously on an estate that has none — and `tfproviderdev`, the instance the
// acceptance lanes run against, has none. So every membership assertion,
// including the one proving Terraform leaves an undeclared membership alone,
// would report success in CI while exercising nothing. That is the exact shape
// TESTING.md exists to prevent: green and zero-coverage look identical from
// outside.
//
// These helpers mint a device inventory record instead, so the coverage does not
// depend on somebody having seeded the estate and survives the next time it is
// cleaned out. Both register their own cleanup.
//
// Three wire facts shape them, each probed on Jamf Pro 11.32.0 on 2026-09-17:
//
//   - A minted record is UNMANAGED unless the create says otherwise, and a
//     static COMPUTER group refuses an unmanaged computer with the same
//     400 INVALID_DEVICE it uses for a device that does not exist. So the
//     computer create sets remote_management.managed itself, which works in one
//     call and needs no follow-up write — there is no classic computer update in
//     the SDK to make one with.
//   - A static MOBILE DEVICE group accepts an unmanaged device without
//     complaint, so the mobile fixture needs nothing extra. Setting managed on a
//     mobile record answers 500 anyway.
//   - An unmanaged record is invisible to smart groups, and the pro lane runs
//     serially, so a fixture cannot perturb another suite's membership counts
//     while it exists.

// MintComputerFixture creates a managed computer inventory record and returns its
// Jamf Pro id, deleting it when the test finishes.
//
// The record is a shell rather than an enrolled Mac, which is all a static
// group's membership needs: the group stores an identifier and Jamf Pro checks
// only that the computer exists, is managed, and is in the group's site.
//
// The create reports no identifier, and the SDK has no by-serial computer read,
// so the id comes from an inventory lookup filtered on the serial this function
// generated. The serial is unique per run, so the lookup cannot match another
// test's fixture.
func MintComputerFixture(t *testing.T, label string) string {
	t.Helper()

	base := NewAcceptanceClient(t)
	classic := proclassic.New(base)
	proClient := pro.New(base)
	ctx := context.Background()

	suffix := RunSuffix()
	name := fmt.Sprintf("tf-acc-%s-mac-%s", label, suffix)
	serial := fixtureSerial("C", suffix)
	udid := fixtureUDID(serial)
	managed := true

	if err := classic.CreateComputerByID(ctx, "0", &proclassic.ComputerPost{
		General: &proclassic.ComputerPostGeneral{
			Name:         &name,
			SerialNumber: &serial,
			UDID:         &udid,
			RemoteManagement: &proclassic.ComputerPostGeneralRemoteManagement{
				Managed: &managed,
			},
		},
	}); err != nil {
		t.Fatalf("minting a computer fixture for %s: %v", label, err)
	}

	t.Cleanup(func() { deleteComputerBySerial(t, classic, proClient, serial) })

	id, err := computerIDForSerial(ctx, proClient, serial)
	if err != nil {
		t.Fatalf("minting a computer fixture for %s: %v", label, err)
	}
	return id
}

// MintMobileDeviceFixture creates a mobile device inventory record and returns
// its Jamf Pro id, deleting it when the test finishes.
//
// Unlike the computer fixture this one stays unmanaged, because a static mobile
// device group accepts it that way and the write that would change it answers
// 500.
//
// The create returns the record, so no lookup is needed.
func MintMobileDeviceFixture(t *testing.T, label string) string {
	t.Helper()

	classic := proclassic.New(NewAcceptanceClient(t))
	ctx := context.Background()

	suffix := RunSuffix()
	name := fmt.Sprintf("tf-acc-%s-ipad-%s", label, suffix)
	serial := fixtureSerial("M", suffix)
	udid := fixtureMobileUDID(serial)

	created, err := classic.CreateMobileDeviceByID(ctx, "0", &proclassic.MobileDevicePost{
		General: &proclassic.MobileDevicePostGeneral{
			Name:         &name,
			SerialNumber: &serial,
			UDID:         &udid,
		},
	})
	if err != nil {
		t.Fatalf("minting a mobile device fixture for %s: %v", label, err)
	}

	t.Cleanup(func() {
		if err := classic.DeleteMobileDeviceBySerialNumber(context.Background(), serial); err != nil {
			t.Errorf("leaving mobile device fixture %s behind on the tenant: %v", serial, err)
		}
	})

	if created == nil || created.ID == nil {
		t.Fatalf("minting a mobile device fixture for %s returned no identifier", label)
	}
	return strconv.Itoa(*created.ID)
}

// fixtureOrdinal distinguishes fixtures minted within one run.
//
// RunSuffix is a sync.OnceValue, so it is the same string for every call in a
// run. Jamf Pro treats a serial number as the identity of a device record, so
// two fixtures built from the suffix alone would share a serial and the second
// create would silently UPDATE the first record rather than make a new one — a
// test asking for two devices would get one id twice and its membership
// assertions would be meaningless. Every fixture therefore takes a distinct
// ordinal.
var fixtureOrdinal atomic.Uint64

// fixtureSerial builds a serial that cannot collide with a real device, with
// another run's fixture, or with another fixture in the same run.
//
// A serial is capped at twelve characters, which is the shape Jamf Pro expects,
// so the run suffix is truncated from the left to keep the ordinal that makes it
// unique.
func fixtureSerial(kind, suffix string) string {
	n := fixtureOrdinal.Add(1)
	tail := kind + strconv.FormatUint(n, 36)
	room := max(12-len("TFACC")-len(tail), 0)
	if len(suffix) > room {
		suffix = suffix[len(suffix)-room:]
	}
	return "TFACC" + suffix + tail
}

// fixtureUDID builds a UUID-shaped identifier for a computer fixture. It carries
// the serial, which is already unique per fixture, because a UDID identifies a
// record just as a serial does.
func fixtureUDID(serial string) string {
	padded := (serial + "000000000000")[:12]
	return "TFACCF00-0000-0000-0000-" + padded
}

// fixtureMobileUDID builds the longer identifier a mobile device record carries,
// which is a different shape from a computer's. It carries the serial for the
// same reason fixtureUDID does.
func fixtureMobileUDID(serial string) string {
	padded := (serial + "0000000000000000000000000000000000")[:34]
	return "TFACCM" + padded
}

// computerIDForSerial resolves a minted computer's Jamf Pro identifier.
//
// The classic create reports no identifier and the SDK has no by-serial computer
// read, so the id comes from an inventory lookup on the serial the caller
// generated. That serial is unique per fixture, so exactly one record can match
// and anything else is a real failure rather than something to pick from.
func computerIDForSerial(ctx context.Context, proClient *pro.Client, serial string) (string, error) {
	found, err := proClient.ListComputersInventoryV4(ctx,
		[]string{pro.ComputerSectionV4General, pro.ComputerSectionV4Hardware}, nil,
		fmt.Sprintf("hardware.serialNumber==%q", serial),
	)
	if err != nil {
		return "", fmt.Errorf("looking up the computer fixture %s by serial: %w", serial, err)
	}
	if len(found) != 1 || found[0].ID == "" {
		return "", fmt.Errorf("looking up the computer fixture %s by serial matched %d records, want exactly 1", serial, len(found))
	}
	return found[0].ID, nil
}

// deleteComputerBySerial removes a minted computer, resolving its identifier
// itself.
//
// The cleanup is registered on the SERIAL rather than on the identifier, and
// before the identifier has been resolved, because the record exists from the
// moment the create returns. Registering it afterwards means a failed lookup
// leaves the record behind — which is exactly what happened the first time this
// was written, and it took two fixture devices with it.
func deleteComputerBySerial(t *testing.T, classic *proclassic.Client, proClient *pro.Client, serial string) {
	t.Helper()
	ctx := context.Background()
	id, err := computerIDForSerial(ctx, proClient, serial)
	if err != nil {
		t.Errorf("leaving computer fixture %s behind on the tenant: %v", serial, err)
		return
	}
	if err := classic.DeleteComputerByID(ctx, id); err != nil {
		t.Errorf("leaving computer fixture %s (%s) behind on the tenant: %v", id, serial, err)
	}
}
