// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// adeSyncTimeout bounds the wait for the shared instance's first Apple sync.
// The jamfplatform_pro_automated_device_enrollment resource defaults to five
// minutes; the shared instance is created once per package, so it can afford
// to be patient.
const adeSyncTimeout = 10 * time.Minute

// adeSyncStateSuccessful is the terminal-success SyncState; the SDK declares no
// constant for it (the resource restates it for the same reason).
const adeSyncStateSuccessful = "SUCCESSFUL"

// adeSyncPollInterval matches the resource's own poll cadence.
const adeSyncPollInterval = 5 * time.Second

var (
	sharedADE   = map[string]*sharedADEInstance{}
	sharedADEMu sync.Mutex
)

type sharedADEInstance struct {
	once sync.Once
	id   string
	err  error
}

// RequireSharedADEInstance returns the id of one Automated Device Enrollment
// instance that every test in the calling package shares, creating it on first
// use from JAMFPLATFORM_ACC_PRO_DEP_TOKEN. label namespaces the instance so two
// packages in one run never share a name.
//
// It exists because the instance's first sync is the dominant cost of a PreStage
// acceptance test: the server holds every upload for about two minutes before
// the sync reports SUCCESSFUL, and a test that uploads its own instance pays
// that wait again. PreStage tests only need an instance to attach to, so they
// share one. Upload, rotation and deletion of an instance stay covered by the
// automated_device_enrollment package's own tests.
//
// The test is skipped when the token is unset, exactly as before. The instance
// is deleted by ReleaseSharedFixtures, which the package's TestMain must
// call after m.Run; a run killed before then leaves an instance named
// "tf-acc-shared-ade-<label>-<run suffix>" on the tenant.
func RequireSharedADEInstance(t *testing.T, label string) string {
	t.Helper()
	token := AccEnv("JAMFPLATFORM_ACC_PRO_DEP_TOKEN")
	if token == "" {
		t.Skip("JAMFPLATFORM_ACC_PRO_DEP_TOKEN not set — acc tests upload a real ADE server token (.p7m base64) and share one ADE instance.")
	}

	sharedADEMu.Lock()
	inst, ok := sharedADE[label]
	if !ok {
		inst = &sharedADEInstance{}
		sharedADE[label] = inst
	}
	sharedADEMu.Unlock()

	inst.once.Do(func() {
		inst.id, inst.err = createSharedADEInstance(NewAcceptanceClient(t), label, token)
	})
	if inst.err != nil {
		t.Fatalf("creating shared ADE instance %q: %v", label, inst.err)
	}
	return inst.id
}

func createSharedADEInstance(c *jamfplatform.Client, label, token string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return "", fmt.Errorf("JAMFPLATFORM_ACC_PRO_DEP_TOKEN is not valid base64: %w", err)
	}
	client := pro.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), adeSyncTimeout)
	defer cancel()

	up, err := client.UploadDeviceEnrollmentTokenV1(ctx, &pro.DeviceEnrollmentToken{EncodedToken: &decoded})
	if err != nil {
		return "", fmt.Errorf("uploading token: %w", err)
	}
	if up == nil || up.ID == "" {
		return "", fmt.Errorf("token upload returned no instance id")
	}
	id := up.ID

	fail := func(err error) (string, error) {
		if delErr := client.DeleteDeviceEnrollmentV1(context.Background(), id); delErr != nil {
			err = fmt.Errorf("%w (rollback delete of %s also failed: %v)", err, id, delErr)
		}
		return "", err
	}

	name := "tf-acc-shared-ade-" + label + "-" + RunSuffix()
	if _, err := client.UpdateDeviceEnrollmentV1(ctx, id, &pro.DeviceEnrollmentInstance{Name: name}); err != nil {
		return fail(fmt.Errorf("naming instance: %w", err))
	}

	ticker := time.NewTicker(adeSyncPollInterval)
	defer ticker.Stop()
	for {
		sync, err := client.GetLatestDeviceEnrollmentSyncV1(ctx, id)
		if err == nil && sync != nil {
			switch {
			case sync.SyncState == adeSyncStateSuccessful:
				return id, nil
			case strings.Contains(sync.SyncState, "FAIL"):
				return fail(fmt.Errorf("first sync failed: state %q", sync.SyncState))
			}
		}
		select {
		case <-ctx.Done():
			return fail(fmt.Errorf("first sync did not reach SUCCESSFUL within %s", adeSyncTimeout))
		case <-ticker.C:
		}
	}
}

// ReleaseSharedFixtures deletes every shared ADE instance and Volume Purchasing
// location created in this process. Call it from the package's TestMain after
// m.Run. Failures are reported on stderr and never change the exit code.
func ReleaseSharedFixtures() {
	releaseSharedVPPLocations()
	sharedADEMu.Lock()
	defer sharedADEMu.Unlock()
	if len(sharedADE) == 0 {
		return
	}
	c, err := initAcceptanceClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "shared ADE release: no client: %v\n", err)
		return
	}
	client := pro.New(c)
	for label, inst := range sharedADE {
		if inst.id == "" {
			continue
		}
		if err := client.DeleteDeviceEnrollmentV1(context.Background(), inst.id); err != nil {
			fmt.Fprintf(os.Stderr, "shared ADE release: deleting %q (%s): %v\n", label, inst.id, err)
		}
	}
}
