// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

const (
	vppSyncTimeout      = 10 * time.Minute
	vppSyncPollInterval = 5 * time.Second
)

var (
	sharedVPP   = map[string]*sharedVPPLocation{}
	sharedVPPMu sync.Mutex
)

type sharedVPPLocation struct {
	once sync.Once
	id   string
	err  error
}

// RequireSharedVPPLocation returns the id of one Volume Purchasing location
// that every test in the calling package shares, creating it on first use from
// JAMFPLATFORM_ACC_PRO_VPP_TOKEN. label namespaces it per package.
//
// It exists for the same reason as RequireSharedADEInstance: creating a location
// blocks until Apple's first content sync reports a lastSyncTime, and a test
// that stands up its own location pays that wait again. Consumers (VPP
// assignments and invitations) only need a synced location to attach to, so they
// reference it through the volume purchasing location data source.
//
// A VPP token registers with one location at a time, so a package that shares a
// location must not also create one through the resource; the location package
// keeps creating its own. Packages run one after another (-p=1), so the two never
// hold the token at once. autoRegisterManagedUsers mirrors the attribute of the
// same name on the resource; the location is always created with
// automatically_populate_purchased_content = true.
//
// Skips when the token is unset. ReleaseSharedFixtures deletes it from TestMain.
func RequireSharedVPPLocation(t *testing.T, label string, autoRegisterManagedUsers bool) string {
	t.Helper()
	token := AccEnv("JAMFPLATFORM_ACC_PRO_VPP_TOKEN")
	if token == "" {
		t.Skip("JAMFPLATFORM_ACC_PRO_VPP_TOKEN not set — acc tests upload a real VPP service token and share one location.")
	}

	sharedVPPMu.Lock()
	loc, ok := sharedVPP[label]
	if !ok {
		loc = &sharedVPPLocation{}
		sharedVPP[label] = loc
	}
	sharedVPPMu.Unlock()

	loc.once.Do(func() {
		loc.id, loc.err = createSharedVPPLocation(NewAcceptanceClient(t), label, token, autoRegisterManagedUsers)
	})
	if loc.err != nil {
		t.Fatalf("creating shared VPP location %q: %v", label, loc.err)
	}
	return loc.id
}

// createSharedVPPLocation creates the location and waits for its first content
// sync. It reclaims licenses after the create and ignores a failure, as the
// resource does: the call clears clientContextMismatch for a token previously
// bound to another Jamf Pro instance and is not needed otherwise.
func createSharedVPPLocation(c *jamfplatform.Client, label, token string, autoRegister bool) (string, error) {
	client := pro.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), vppSyncTimeout)
	defer cancel()

	name := "tf-acc-shared-vpp-" + label + "-" + RunSuffix()
	populate := true
	created, err := client.CreateVolumePurchasingLocationV1(ctx, &pro.VolumePurchasingLocationPost{
		Name:                                  &name,
		ServiceToken:                          strings.TrimSpace(token),
		AutomaticallyPopulatePurchasedContent: &populate,
		AutoRegisterManagedUsers:              &autoRegister,
	})
	if err != nil {
		return "", fmt.Errorf("creating location: %w", err)
	}
	if created == nil || created.ID == "" {
		return "", fmt.Errorf("location create returned no id")
	}
	id := created.ID

	fail := func(err error) (string, error) {
		if delErr := client.DeleteVolumePurchasingLocationV1(context.Background(), id); delErr != nil {
			err = fmt.Errorf("%w (rollback delete of %s also failed: %v)", err, id, delErr)
		}
		return "", err
	}

	_ = client.ReclaimVolumePurchasingLocationLicensesV1(ctx, id)

	ticker := time.NewTicker(vppSyncPollInterval)
	defer ticker.Stop()
	for {
		loc, err := client.GetVolumePurchasingLocationV1(ctx, id)
		if err != nil {
			return fail(fmt.Errorf("polling first sync: %w", err))
		}
		if loc != nil && loc.LastSyncTime != "" {
			return id, nil
		}
		select {
		case <-ctx.Done():
			return fail(fmt.Errorf("first content sync did not complete within %s", vppSyncTimeout))
		case <-ticker.C:
		}
	}
}

func releaseSharedVPPLocations() {
	sharedVPPMu.Lock()
	defer sharedVPPMu.Unlock()
	if len(sharedVPP) == 0 {
		return
	}
	c, err := initAcceptanceClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "shared VPP release: no client: %v\n", err)
		return
	}
	client := pro.New(c)
	for label, loc := range sharedVPP {
		if loc.id == "" {
			continue
		}
		if err := client.DeleteVolumePurchasingLocationV1(context.Background(), loc.id); err != nil {
			fmt.Fprintf(os.Stderr, "shared VPP release: deleting %q (%s): %v\n", label, loc.id, err)
		}
	}
}
