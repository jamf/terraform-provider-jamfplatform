// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_external_source

import (
	"encoding/xml"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestPatchExternalSource_LiveWireUnmarshal pins the decoding contract for the
// real /patchexternalsources wire shape (with an empty <port/> element). The
// XML decoder unmarshals <port/> to a non-nil *int(0) — NOT nil — which is why
// portValueOrNull collapses both nil and 0 to a null Int64. If the SDK ever
// changes this, the read mapping must be revisited.
func TestPatchExternalSource_LiveWireUnmarshal(t *testing.T) {
	wire := `<patch_external_source><name>Jamf Auto Update</name><port/><host_name>definitions.datajar.mobi/v2/</host_name><ssl_enabled>true</ssl_enabled><certificate_validation_enabled>false</certificate_validation_enabled><enabled>true</enabled><id>2</id></patch_external_source>`

	var p proclassic.PatchExternalSource
	if err := xml.Unmarshal([]byte(wire), &p); err != nil {
		t.Fatalf("unmarshal live wire: %v", err)
	}
	if p.Port == nil {
		t.Fatalf("expected non-nil Port pointer for empty <port/> (decoder yields *int(0))")
	}
	if *p.Port != 0 {
		t.Errorf("expected empty <port/> to decode to 0, got %d", *p.Port)
	}
	if p.Name == nil || *p.Name != "Jamf Auto Update" {
		t.Errorf("Name mismatch: %v", p.Name)
	}
	if p.HostName == nil || *p.HostName != "definitions.datajar.mobi/v2/" {
		t.Errorf("HostName mismatch: %v", p.HostName)
	}
	if p.SslEnabled == nil || *p.SslEnabled != true {
		t.Errorf("SslEnabled mismatch: %v", p.SslEnabled)
	}
	if p.CertificateValidationEnabled == nil || *p.CertificateValidationEnabled != false {
		t.Errorf("CertificateValidationEnabled mismatch: %v", p.CertificateValidationEnabled)
	}
	if p.Enabled == nil || *p.Enabled != true {
		t.Errorf("Enabled mismatch: %v", p.Enabled)
	}
	if p.ID == nil || *p.ID != 2 {
		t.Errorf("ID mismatch: %v", p.ID)
	}
}

func TestPortValueOrNull(t *testing.T) {
	zero := 0
	port := 8443
	cases := []struct {
		name    string
		in      *int
		wantNil bool
		want    int64
	}{
		{name: "nil pointer", in: nil, wantNil: true},
		{name: "zero collapses to null", in: &zero, wantNil: true},
		{name: "real port", in: &port, wantNil: false, want: 8443},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := portValueOrNull(tc.in)
			if tc.wantNil {
				if !got.IsNull() {
					t.Errorf("expected null, got %v", got)
				}
				return
			}
			if got.IsNull() {
				t.Fatalf("expected %d, got null", tc.want)
			}
			if got.ValueInt64() != tc.want {
				t.Errorf("expected %d, got %d", tc.want, got.ValueInt64())
			}
		})
	}
}

func TestAssignPatchExternalSourceResourceModel_FullPopulate(t *testing.T) {
	id, name, host, port := 7, "Source", "host.example", 9000
	enabled, ssl, cert := true, false, true
	state := PatchExternalSourceResourceModel{}
	api := &proclassic.PatchExternalSource{
		ID:                           &id,
		Name:                         &name,
		Enabled:                      &enabled,
		HostName:                     &host,
		Port:                         &port,
		SslEnabled:                   &ssl,
		CertificateValidationEnabled: &cert,
	}

	assignPatchExternalSourceResourceModel(&state, api)

	if state.ID.ValueString() != "7" {
		t.Errorf("ID: got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Source" {
		t.Errorf("Name: got %q", state.Name.ValueString())
	}
	if state.Enabled.ValueBool() != true {
		t.Errorf("Enabled: got %v", state.Enabled.ValueBool())
	}
	if state.HostName.ValueString() != "host.example" {
		t.Errorf("HostName: got %q", state.HostName.ValueString())
	}
	if state.Port.ValueInt64() != 9000 {
		t.Errorf("Port: got %d", state.Port.ValueInt64())
	}
	if state.SslEnabled.ValueBool() != false {
		t.Errorf("SslEnabled: got %v", state.SslEnabled.ValueBool())
	}
	if state.CertificateValidationEnabled.ValueBool() != true {
		t.Errorf("CertificateValidationEnabled: got %v", state.CertificateValidationEnabled.ValueBool())
	}
}

// TestAssignPatchExternalSourceResourceModel_EmptyPortBecomesNull mirrors a read
// after Create where the server echoed <port/> (decoded to *int(0)).
func TestAssignPatchExternalSourceResourceModel_EmptyPortBecomesNull(t *testing.T) {
	id, name, zero := 3, "NoPort", 0
	state := PatchExternalSourceResourceModel{}
	api := &proclassic.PatchExternalSource{
		ID:   &id,
		Name: &name,
		Port: &zero,
	}
	assignPatchExternalSourceResourceModel(&state, api)
	if !state.Port.IsNull() {
		t.Errorf("expected Port null for echoed 0, got %d", state.Port.ValueInt64())
	}
}

func TestAssignPatchExternalSourceResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	name := "refreshed"
	state := PatchExternalSourceResourceModel{ID: types.StringValue("42")}
	api := &proclassic.PatchExternalSource{ID: nil, Name: &name}

	assignPatchExternalSourceResourceModel(&state, api)

	if state.ID.ValueString() != "42" {
		t.Errorf("expected ID preserved as 42, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "refreshed" {
		t.Errorf("expected Name refreshed, got %q", state.Name.ValueString())
	}
}

func TestAssignPatchExternalSourceResourceModel_NilAPIIsNoop(t *testing.T) {
	state := PatchExternalSourceResourceModel{
		ID:   types.StringValue("7"),
		Name: types.StringValue("Keep"),
	}
	assignPatchExternalSourceResourceModel(&state, nil)
	if state.ID.ValueString() != "7" || state.Name.ValueString() != "Keep" {
		t.Errorf("expected state unchanged, got id=%q name=%q", state.ID.ValueString(), state.Name.ValueString())
	}
}

func TestAssignPatchExternalSourceDataSourceModel_PopulatesAll(t *testing.T) {
	id, name, host, port := 11, "Looked Up", "h", 443
	enabled, ssl, cert := true, true, false
	state := PatchExternalSourceDataSourceModel{}
	api := &proclassic.PatchExternalSource{
		ID:                           &id,
		Name:                         &name,
		Enabled:                      &enabled,
		HostName:                     &host,
		Port:                         &port,
		SslEnabled:                   &ssl,
		CertificateValidationEnabled: &cert,
	}

	assignPatchExternalSourceDataSourceModel(&state, api)

	if state.ID.ValueString() != "11" {
		t.Errorf("ID: got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Looked Up" {
		t.Errorf("Name: got %q", state.Name.ValueString())
	}
	if state.Port.ValueInt64() != 443 {
		t.Errorf("Port: got %d", state.Port.ValueInt64())
	}
	if state.SslEnabled.ValueBool() != true {
		t.Errorf("SslEnabled: got %v", state.SslEnabled.ValueBool())
	}
}

func TestAssignPatchExternalSourceDataSourceModel_PreservesSelectorOnNilAPIFields(t *testing.T) {
	id := 7
	state := PatchExternalSourceDataSourceModel{
		ID:   types.StringNull(),
		Name: types.StringValue("Primary"),
	}
	api := &proclassic.PatchExternalSource{ID: &id, Name: nil}

	assignPatchExternalSourceDataSourceModel(&state, api)

	if state.ID.ValueString() != "7" {
		t.Errorf("expected ID written, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Primary" {
		t.Errorf("expected Name preserved as Primary, got %q", state.Name.ValueString())
	}
}
