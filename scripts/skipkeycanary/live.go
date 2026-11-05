//go:build skipkeycanary

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

const (
	syncPollInterval = 5 * time.Second
	syncSuccessful   = "SUCCESSFUL"
)

type live struct {
	pro *pro.Client
}

func (l live) ListInstances(ctx context.Context) ([]Instance, error) {
	list, err := l.pro.ListDeviceEnrollmentsV1(ctx, nil)
	if err != nil {
		return nil, err
	}
	out := make([]Instance, 0, len(list))
	for _, in := range list {
		id := ""
		if in.ID != nil {
			id = *in.ID
		}
		uuid := ""
		if in.ServerUUID != nil {
			uuid = *in.ServerUUID
		}
		out = append(out, Instance{ID: id, Name: in.Name, ServerUUID: uuid})
	}
	return out, nil
}

func (l live) UploadToken(ctx context.Context, name string, token []byte) (string, error) {
	resp, err := l.pro.UploadDeviceEnrollmentTokenV1(ctx, &pro.DeviceEnrollmentToken{EncodedToken: &token, TokenFileName: &name})
	if err != nil {
		if isAlreadyRegistered(err) {
			return "", fmt.Errorf("%w: %v", ErrAlreadyRegistered, err)
		}
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return resp.ID, nil
}

// isAlreadyRegistered reports whether Jamf Pro refused an upload because an instance already holds
// the token. It accepts HTTP 409 and nothing else. The exact status and error code are unverified
// on the live API: the nmartin tenant created a new instance for every upload of its own token and
// never refused one. A wider test on error text risked sending PreStage writes to an unrelated
// instance, so a refusal with a different shape fails the run instead.
func isAlreadyRegistered(err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	return apiErr != nil && apiErr.HasStatus(http.StatusConflict)
}

func (l live) RenameInstance(ctx context.Context, id, name string) error {
	_, err := l.pro.UpdateDeviceEnrollmentV1(ctx, id, &pro.DeviceEnrollmentInstance{Name: name})
	return err
}

func (l live) WaitInstanceSynced(ctx context.Context, id string) error {
	ticker := time.NewTicker(syncPollInterval)
	defer ticker.Stop()
	for {
		sync, err := l.pro.GetLatestDeviceEnrollmentSyncV1(ctx, id)
		switch {
		case err != nil:
			if apiErr := jamfplatform.AsAPIError(err); apiErr == nil || !apiErr.HasStatus(http.StatusNotFound) {
				return err
			}
		case sync != nil && sync.SyncState == syncSuccessful:
			return nil
		case sync != nil && strings.Contains(sync.SyncState, "FAIL"):
			return fmt.Errorf("sync state %q", sync.SyncState)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("sync did not reach %s: %w", syncSuccessful, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (l live) DeleteInstance(ctx context.Context, id string) error {
	return l.pro.DeleteDeviceEnrollmentV1(ctx, id)
}

func (l live) ListPrestages(ctx context.Context, kind Kind) ([]Prestage, error) {
	switch kind {
	case Computer:
		list, err := l.pro.ListComputerPrestagesV3(ctx, nil)
		if err != nil {
			return nil, err
		}
		out := make([]Prestage, 0, len(list))
		for _, p := range list {
			out = append(out, Prestage{ID: p.ID, Name: p.DisplayName})
		}
		return out, nil
	default:
		list, err := l.pro.ListMobileDevicePrestagesV3(ctx, nil)
		if err != nil {
			return nil, err
		}
		out := make([]Prestage, 0, len(list))
		for _, p := range list {
			out = append(out, Prestage{ID: p.ID, Name: p.DisplayName})
		}
		return out, nil
	}
}

func (l live) CreatePrestage(ctx context.Context, kind Kind, name, instanceID string) (string, error) {
	switch kind {
	case Computer:
		resp, err := l.pro.CreateComputerPrestageV3(ctx, computerBody(name, instanceID))
		if err != nil {
			return "", err
		}
		return resp.ID, nil
	default:
		resp, err := l.pro.CreateMobileDevicePrestageV3(ctx, mobileBody(name, instanceID))
		if err != nil {
			return "", err
		}
		return resp.ID, nil
	}
}

func (l live) PrestageSkipKeys(ctx context.Context, kind Kind, id string) ([]string, error) {
	var items map[string]bool
	switch kind {
	case Computer:
		got, err := l.pro.GetComputerPrestageV3(ctx, id)
		if err != nil {
			return nil, err
		}
		items = got.SkipSetupItems
	default:
		got, err := l.pro.GetMobileDevicePrestageV3(ctx, id)
		if err != nil {
			return nil, err
		}
		items = got.SkipSetupItems
	}
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (l live) DeletePrestage(ctx context.Context, kind Kind, id string) error {
	if kind == Computer {
		return l.pro.DeleteComputerPrestageV3(ctx, id)
	}
	return l.pro.DeleteMobileDevicePrestageV3(ctx, id)
}

func emptyLocation() pro.LocationInformationV3 {
	return pro.LocationInformationV3{BuildingID: "-1", DepartmentID: "-1", ID: "-1"}
}

func emptyPurchasingV3() pro.PrestagePurchasingInformationV3 {
	return pro.PrestagePurchasingInformationV3{
		ID: "-1", Purchased: true,
		LeaseDate: "1970-01-01", PoDate: "1970-01-01", WarrantyDate: "1970-01-01",
	}
}

func computerBody(name, instanceID string) *pro.PostComputerPrestageV3 {
	loc := emptyLocation()
	pur := emptyPurchasingV3()
	return &pro.PostComputerPrestageV3{
		DisplayName:                       name,
		DeviceEnrollmentProgramInstanceID: instanceID,
		EnrollmentSiteID:                  "-1",
		MDMRemovable:                      true,
		CustomPackageDistributionPointID:  "-1",
		CustomPackageIds:                  []string{},
		PrestageInstalledProfileIds:       []string{},
		LocationInformation: pro.LocationInformationV2{
			BuildingID: loc.BuildingID, DepartmentID: loc.DepartmentID, ID: loc.ID,
		},
		PurchasingInformation: pro.PrestagePurchasingInformationV2{
			ID: pur.ID, Purchased: pur.Purchased,
			LeaseDate: pur.LeaseDate, PoDate: pur.PoDate, WarrantyDate: pur.WarrantyDate,
		},
	}
}

func mobileBody(name, instanceID string) *pro.MobileDevicePrestageV3 {
	noEnforcementIos := pro.MobileDevicePrestageV3PrestageMinimumOsTargetVersionTypeIosNoEnforcement
	noEnforcementIpad := pro.MobileDevicePrestageV3PrestageMinimumOsTargetVersionTypeIpadNoEnforcement
	off := false
	empty := ""
	noProfile := "-1"
	noNames := []pro.MobileDevicePrestageNameV3{}
	defaultNames := "Default Names"
	return &pro.MobileDevicePrestageV3{
		DisplayName:                            name,
		DeviceEnrollmentProgramInstanceID:      instanceID,
		EnrollmentSiteID:                       "-1",
		MDMRemovable:                           true,
		Supervised:                             true,
		AllowPairing:                           true,
		MaximumSharedAccounts:                  10,
		StorageQuotaSizeMegabytes:              1024,
		Timezone:                               "UTC",
		LocationInformation:                    emptyLocation(),
		PurchasingInformation:                  emptyPurchasingV3(),
		PrestageMinimumOsTargetVersionTypeIos:  &noEnforcementIos,
		PrestageMinimumOsTargetVersionTypeIpad: &noEnforcementIpad,
		MinimumOsSpecificVersionIos:            &empty,
		MinimumOsSpecificVersionIpad:           &empty,
		RtsEnabled:                             &off,
		RtsConfigProfileID:                     &noProfile,
		PreserveManagedApps:                    &off,
		InstallAppsDuringEnrollment:            &off,
		DoNotUseProfileFromBackup:              &off,
		TemporarySessionOnly:                   &off,
		EnforceTemporarySessionTimeout:         &off,
		EnforceUserSessionTimeout:              &off,
		Names: &pro.MobileDevicePrestageNamesV3{
			AssignNamesUsing:       &defaultNames,
			ManageNames:            &off,
			DeviceNamingConfigured: &off,
			PrestageDeviceNames:    &noNames,
		},
	}
}
