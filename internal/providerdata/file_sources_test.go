// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"testing"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filesources"
)

func TestFileSourcesIsSharedAndNilSafe(t *testing.T) {
	var unconfigured *Data
	if unconfigured.FileSources() != nil {
		t.Error("a nil Data must hand out a nil resolver, not panic")
	}
	if ConfigureFileSources(nil) != nil {
		t.Error("the nil ProviderData of early lifecycle must yield a nil resolver")
	}
	if ConfigureFileSources("not provider data") != nil {
		t.Error("a ProviderData of the wrong type must yield a nil resolver")
	}

	pd := &Data{}
	if pd.FileSources() != nil {
		t.Error("the resolver must be nil until SetFileSources is called")
	}
	resolver := filesources.New(filesources.Config{}, nil)
	pd.SetFileSources(resolver)
	if pd.FileSources() != resolver || ConfigureFileSources(pd) != resolver {
		t.Error("every construct must receive the one resolver the provider built")
	}
}
