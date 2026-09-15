// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// apiErrStub is the minimum interface isPutSerializerBug type-asserts
// against — it only needs HasStatus(int) bool.
type apiErrStub struct {
	status int
	body   string
}

func (e *apiErrStub) Error() string {
	return e.body
}

func (e *apiErrStub) HasStatus(s int) bool {
	return e.status == s
}

func TestIsPutSerializerBug(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "non-API error",
			err:  errors.New("network broken"),
			want: false,
		},
		{
			name: "wrong status",
			err:  &apiErrStub{status: 400, body: `{"httpStatus":400,"errors":[]}`},
			want: false,
		},
		{
			name: "500 with populated errors[] — not the bug",
			err:  &apiErrStub{status: 500, body: `{"httpStatus":500,"errors":[{"code":"INTERNAL"}]}`},
			want: false,
		},
		{
			name: "500 with empty errors[] (compact)",
			err:  &apiErrStub{status: 500, body: `{"httpStatus":500,"errors":[]}`},
			want: true,
		},
		{
			name: "500 with empty errors[] (pretty-printed)",
			err:  &apiErrStub{status: 500, body: `{"httpStatus" : 500, "errors" : [ ]}`},
			want: true,
		},
		{
			name: "wrapped 500-with-empty-errors",
			err:  fmt.Errorf("UpdateComputerPrestageV3(131): API request failed with status 500: %w", &apiErrStub{status: 500, body: `{"httpStatus":500,"errors":[]}`}),
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPutSerializerBug(tc.err); got != tc.want {
				t.Errorf("isPutSerializerBug = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInjectVersionLocks_RootAndNestedEcho(t *testing.T) {
	put := &pro.PutComputerPrestageV3{
		LocationInformation:   pro.LocationInformationV2{},
		PurchasingInformation: pro.PrestagePurchasingInformationV2{},
	}
	got := &pro.GetComputerPrestageV3{
		VersionLock: 7,
		LocationInformation: &pro.LocationInformationV2{
			ID:          "99",
			VersionLock: 3,
		},
		PurchasingInformation: &pro.PrestagePurchasingInformationV2{
			ID:          "101",
			VersionLock: 4,
		},
		AccountSettings: &pro.AccountSettingsResponse{
			ID:          "87",
			VersionLock: 2,
		},
	}

	injectVersionLocks(put, got)

	if put.VersionLock == nil || *put.VersionLock != 7 {
		t.Errorf("root versionLock want 7, got %v", put.VersionLock)
	}
	if put.LocationInformation.ID != "99" || put.LocationInformation.VersionLock != 3 {
		t.Errorf("location echo failed: %+v", put.LocationInformation)
	}
	if put.PurchasingInformation.ID != "101" || put.PurchasingInformation.VersionLock != 4 {
		t.Errorf("purchasing echo failed: %+v", put.PurchasingInformation)
	}
	if put.AccountSettings == nil || put.AccountSettings.ID == nil || *put.AccountSettings.ID != "87" {
		t.Errorf("accountSettings.id echo failed: %+v", put.AccountSettings)
	}
	if put.AccountSettings.VersionLock == nil || *put.AccountSettings.VersionLock != 2 {
		t.Errorf("accountSettings.versionLock echo failed: %v", put.AccountSettings.VersionLock)
	}
}

func TestInjectVersionLocks_NilNestedFromGet(t *testing.T) {
	put := &pro.PutComputerPrestageV3{}
	got := &pro.GetComputerPrestageV3{VersionLock: 1}

	injectVersionLocks(put, got)

	if put.VersionLock == nil || *put.VersionLock != 1 {
		t.Errorf("root lock want 1")
	}
	if put.LocationInformation.ID != "" || put.LocationInformation.VersionLock != 0 {
		t.Errorf("location should reset to zero when GET omits it: %+v", put.LocationInformation)
	}
	if put.PurchasingInformation.ID != "" || put.PurchasingInformation.VersionLock != 0 {
		t.Errorf("purchasing should reset to zero when GET omits it: %+v", put.PurchasingInformation)
	}
	if put.AccountSettings == nil {
		t.Errorf("accountSettings should be auto-initialised on nil GET")
	}
}

func TestFmtUnchangedFields(t *testing.T) {
	if fmtUnchangedFields(nil) != "" {
		t.Errorf("nil slice should produce empty string")
	}
	if got := fmtUnchangedFields([]string{"display_name"}); got == "" {
		t.Errorf("single field should produce non-empty output")
	}
	got := fmtUnchangedFields([]string{"display_name", "account_settings.admin_username"})
	if got == "" {
		t.Errorf("multi-field output empty")
	}
}
