// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// unknownValue marks a value in a test configuration as unknown, the state of
// anything derived from a resource Terraform has not created yet.
type unknownValue struct{}

// tfValue builds a Terraform value of typ from plain Go data: a string, a
// bool, a map for an object, a slice for a list, unknownValue{} for unknown,
// and nil or an absent map key for null. Filling the absent keys is what lets
// a test state only the attributes it is about.
func tfValue(t *testing.T, typ tftypes.Type, value any) tftypes.Value {
	t.Helper()
	switch v := value.(type) {
	case nil:
		return tftypes.NewValue(typ, nil)
	case unknownValue:
		return tftypes.NewValue(typ, tftypes.UnknownValue)
	case map[string]any:
		object, ok := typ.(tftypes.Object)
		if !ok {
			t.Fatalf("a map was given for %s", typ)
		}
		for name := range v {
			if _, known := object.AttributeTypes[name]; !known {
				t.Fatalf("no attribute %q in %s", name, typ)
			}
		}
		attributes := make(map[string]tftypes.Value, len(object.AttributeTypes))
		for name, attributeType := range object.AttributeTypes {
			attributes[name] = tfValue(t, attributeType, v[name])
		}
		return tftypes.NewValue(typ, attributes)
	case []any:
		list, ok := typ.(tftypes.List)
		if !ok {
			t.Fatalf("a slice was given for %s", typ)
		}
		elements := make([]tftypes.Value, len(v))
		for i, element := range v {
			elements[i] = tfValue(t, list.ElementType, element)
		}
		return tftypes.NewValue(typ, elements)
	default:
		return tftypes.NewValue(typ, value)
	}
}

func providerSchema(t *testing.T) schema.Schema {
	t.Helper()
	var resp provider.SchemaResponse
	New("test")().Schema(context.Background(), provider.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("provider schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// validateFileSources runs the provider's whole configuration validation, as
// Terraform does, over a configuration holding only the given file_sources
// value, and returns the error diagnostics.
func validateFileSources(t *testing.T, fileSources any) []*tfprotov6.Diagnostic {
	t.Helper()
	ctx := context.Background()
	configType := providerSchema(t).Type().TerraformType(ctx)
	config, err := tfprotov6.NewDynamicValue(configType, tfValue(t, configType, map[string]any{"file_sources": fileSources}))
	if err != nil {
		t.Fatalf("encoding the configuration: %v", err)
	}
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatalf("building the provider server: %v", err)
	}
	resp, err := server.ValidateProviderConfig(ctx, &tfprotov6.ValidateProviderConfigRequest{Config: &config})
	if err != nil {
		t.Fatalf("validating: %v", err)
	}
	var errs []*tfprotov6.Diagnostic
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			errs = append(errs, d)
		}
	}
	return errs
}

func fileSourcesObject(t *testing.T, value any) types.Object {
	t.Helper()
	ctx := context.Background()
	attribute := providerSchema(t).Attributes["file_sources"]
	typ := attribute.GetType()
	decoded, err := typ.ValueFromTerraform(ctx, tfValue(t, typ.TerraformType(ctx), value))
	if err != nil {
		t.Fatalf("decoding file_sources: %v", err)
	}
	object, ok := decoded.(types.Object)
	if !ok {
		t.Fatalf("file_sources decoded as %T", decoded)
	}
	return object
}

func TestFileSourcesValidation(t *testing.T) {
	cases := []struct {
		name        string
		fileSources any
		wantErrorAt string
	}{
		{name: "absent"},
		{name: "empty block", fileSources: map[string]any{}},
		{name: "wholly unknown", fileSources: unknownValue{}},
		{name: "empty nested blocks", fileSources: map[string]any{
			"s3": map[string]any{}, "azure": map[string]any{}, "https": []any{},
		}},

		{name: "s3 profile and region", fileSources: map[string]any{"s3": map[string]any{
			"region": "eu-west-1", "profile": "packages",
		}}},
		{name: "s3 static keys with a token", fileSources: map[string]any{"s3": map[string]any{
			"access_key": "AKIAEXAMPLE", "secret_key": "x", "token": "y",
		}}},
		{name: "s3 access key alone", wantErrorAt: "secret_key", fileSources: map[string]any{"s3": map[string]any{
			"access_key": "AKIAEXAMPLE",
		}}},
		{name: "s3 secret key alone", wantErrorAt: "access_key", fileSources: map[string]any{"s3": map[string]any{
			"secret_key": "x",
		}}},
		{name: "s3 token alone", wantErrorAt: "access_key", fileSources: map[string]any{"s3": map[string]any{
			"token": "y",
		}}},
		{name: "s3 access key with an unknown secret", fileSources: map[string]any{"s3": map[string]any{
			"access_key": "AKIAEXAMPLE", "secret_key": unknownValue{},
		}}},
		{name: "s3 unknown block", fileSources: map[string]any{"s3": unknownValue{}}},
		{name: "s3 assume role", fileSources: map[string]any{"s3": map[string]any{
			"assume_role": map[string]any{"role_arn": "arn:aws:iam::123456789012:role/pkg-read", "external_id": "x"},
		}}},
		{name: "s3 assume role without an arn", wantErrorAt: "role_arn", fileSources: map[string]any{"s3": map[string]any{
			"assume_role": map[string]any{"session_name": "jamfplatform"},
		}}},
		{name: "s3 web identity with a token file", fileSources: map[string]any{"s3": map[string]any{
			"assume_role_with_web_identity": map[string]any{"role_arn": "arn:aws:iam::123456789012:role/pkg-read", "web_identity_token_file": "/var/run/token"},
		}}},
		{name: "s3 web identity with the token from the environment", fileSources: map[string]any{"s3": map[string]any{
			"assume_role_with_web_identity": map[string]any{"role_arn": "arn:aws:iam::123456789012:role/pkg-read"},
		}}},
		{name: "s3 web identity with both token forms", wantErrorAt: "web_identity_token", fileSources: map[string]any{"s3": map[string]any{
			"assume_role_with_web_identity": map[string]any{"role_arn": "arn:aws:iam::123456789012:role/pkg-read", "web_identity_token": "t", "web_identity_token_file": "/var/run/token"},
		}}},
		{name: "s3 compatible endpoint", fileSources: map[string]any{"s3": map[string]any{
			"endpoints": map[string]any{"s3": "https://minio.internal:9000"}, "use_path_style": true,
		}}},

		{name: "azure oidc with the client id from the environment", fileSources: map[string]any{"azure": map[string]any{
			"use_oidc": true,
		}}},
		{name: "azure client secret with the client id from the environment", fileSources: map[string]any{"azure": map[string]any{
			"client_secret": "x",
		}}},
		{name: "azure both secret forms", wantErrorAt: "client_secret", fileSources: map[string]any{"azure": map[string]any{
			"client_secret": "x", "client_secret_file_path": "/run/secret",
		}}},
		{name: "azure both oidc token forms", wantErrorAt: "oidc_token", fileSources: map[string]any{"azure": map[string]any{
			"use_oidc": true, "oidc_token": "t", "oidc_token_file_path": "/run/token",
		}}},
		{name: "azure sovereign cloud", fileSources: map[string]any{"azure": map[string]any{
			"environment": "usgovernment",
		}}},
		{name: "azure unrecognised cloud", wantErrorAt: "environment", fileSources: map[string]any{"azure": map[string]any{
			"environment": "germany",
		}}},
		{name: "azure unknown cloud defers", fileSources: map[string]any{"azure": map[string]any{
			"environment": unknownValue{},
		}}},

		{name: "https one of each form", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "artifactory.example.com", "bearer_token": "t"},
			map[string]any{"host": "files.example.com", "username": "ci", "password": "p"},
			map[string]any{"host": "dl.example.com:8443", "header": map[string]any{"name": "X-Api-Key", "value": "k"}},
		}}},
		{name: "https entry with no credential", wantErrorAt: "bearer_token", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com"},
		}}},
		{name: "https bearer token and basic", wantErrorAt: "bearer_token", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "bearer_token": "t", "username": "ci", "password": "p"},
		}}},
		{name: "https bearer token and header", wantErrorAt: "bearer_token", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "bearer_token": "t", "header": map[string]any{"name": "X-Api-Key", "value": "k"}},
		}}},
		{name: "https user name without a password", wantErrorAt: "password", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "username": "ci"},
		}}},
		{name: "https password without a user name", wantErrorAt: "username", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "password": "p"},
		}}},
		{name: "https unknown credential defers", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "bearer_token": unknownValue{}},
		}}},
		{name: "https user name with an unknown password defers", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "username": "ci", "password": unknownValue{}},
		}}},
		{name: "https unknown entry defers", fileSources: map[string]any{"https": []any{unknownValue{}}}},
		{name: "https entry without a host", wantErrorAt: "host", fileSources: map[string]any{"https": []any{
			map[string]any{"bearer_token": "t"},
		}}},
		{name: "https wildcard host", wantErrorAt: "host", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "*.example.com", "bearer_token": "t"},
		}}},
		{name: "https one ipv6 address spelled twice", wantErrorAt: "host", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "[::1]", "bearer_token": "t"},
			map[string]any{"host": "[0:0:0:0:0:0:0:0001]:443", "bearer_token": "u"},
		}}},
		{name: "https unknown list defers", fileSources: map[string]any{"https": unknownValue{}}},
		{name: "https host with a scheme", wantErrorAt: "host", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "https://files.example.com", "bearer_token": "t"},
		}}},
		{name: "https host with a path", wantErrorAt: "host", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com/packages", "bearer_token": "t"},
		}}},
		{name: "https unknown host defers", fileSources: map[string]any{"https": []any{
			map[string]any{"host": unknownValue{}, "bearer_token": "t"},
		}}},
		{name: "https the same host twice", wantErrorAt: "host", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "bearer_token": "t"},
			map[string]any{"host": "FILES.example.com:443", "bearer_token": "u"},
		}}},
		{name: "https one host on two ports", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "bearer_token": "t"},
			map[string]any{"host": "files.example.com:8443", "bearer_token": "u"},
		}}},
		{name: "https empty bearer token", wantErrorAt: "bearer_token", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "bearer_token": ""},
		}}},
		{name: "https empty password", wantErrorAt: "password", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "username": "ci", "password": ""},
		}}},
		{name: "https empty header value", wantErrorAt: "value", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "header": map[string]any{"name": "X-Api-Key", "value": ""}},
		}}},
		{name: "https header the client manages", wantErrorAt: "name", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "header": map[string]any{"name": "User-Agent", "value": "k"}},
		}}},
		{name: "https header the client withholds", wantErrorAt: "name", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "header": map[string]any{"name": "host", "value": "k"}},
		}}},
		{name: "https credential in a named authorization header", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "header": map[string]any{"name": "Authorization", "value": "Token k"}},
		}}},
		{name: "https host that is not ascii", wantErrorAt: "host", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "f\u0130les.example.com", "bearer_token": "t"},
		}}},
		{name: "https header name with a space", wantErrorAt: "name", fileSources: map[string]any{"https": []any{
			map[string]any{"host": "files.example.com", "header": map[string]any{"name": "X Api Key", "value": "k"}},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateFileSources(t, tc.fileSources)
			if tc.wantErrorAt == "" {
				for _, d := range errs {
					t.Errorf("unexpected error at %s: %s: %s", d.Attribute, d.Summary, d.Detail)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("want an error at %s, got none", tc.wantErrorAt)
			}
			found := false
			for _, d := range errs {
				steps := d.Attribute.Steps()
				last, _ := steps[len(steps)-1].(tftypes.AttributeName)
				found = found || string(last) == tc.wantErrorAt || strings.Contains(d.Detail, tc.wantErrorAt)
			}
			if !found {
				for _, d := range errs {
					t.Errorf("error at %s: %s: %s", d.Attribute, d.Summary, d.Detail)
				}
				t.Fatalf("no error names %s", tc.wantErrorAt)
			}
		})
	}
}

func TestFileSourcesSecretsAreSensitive(t *testing.T) {
	want := []string{
		"azure.client_secret",
		"azure.oidc_request_token",
		"azure.oidc_token",
		"https.bearer_token",
		"https.header.value",
		"https.password",
		"s3.access_key",
		"s3.assume_role_with_web_identity.web_identity_token",
		"s3.secret_key",
		"s3.token",
	}
	var got []string
	var walk func(prefix string, attributes map[string]schema.Attribute)
	walk = func(prefix string, attributes map[string]schema.Attribute) {
		for name, attribute := range attributes {
			at := prefix + name
			if attribute.IsSensitive() {
				got = append(got, at)
			}
			switch nested := attribute.(type) {
			case schema.SingleNestedAttribute:
				walk(at+".", nested.Attributes)
			case schema.ListNestedAttribute:
				walk(at+".", nested.NestedObject.Attributes)
			}
		}
	}
	block, ok := providerSchema(t).Attributes["file_sources"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("file_sources is not a single nested attribute")
	}
	walk("", block.Attributes)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("sensitive attributes:\n got %v\nwant %v", got, want)
	}
}

func TestFileSourcesDescriptionNamesItsOneConsumer(t *testing.T) {
	description := providerSchema(t).Attributes["file_sources"].GetMarkdownDescription()
	for _, want := range []string{"`package_file_source`", "`jamfplatform_pro_package`", "no other attribute"} {
		if !strings.Contains(description, want) {
			t.Errorf("the file_sources description must say %s", want)
		}
	}
}

func TestResolveFileSourcesAbsent(t *testing.T) {
	config, unknown, diags := resolveFileSources(context.Background(), fileSourcesObject(t, nil))
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(unknown) != 0 {
		t.Errorf("unknown = %v", unknown)
	}
	if config.S3.AssumeRole != nil || config.Azure.UseCLI != nil || len(config.HTTPS) != 0 || config.S3.Region != "" {
		t.Error("an absent block must resolve to the zero configuration")
	}
}

func TestResolveFileSourcesReadsEveryValue(t *testing.T) {
	config, unknown, diags := resolveFileSources(context.Background(), fileSourcesObject(t, map[string]any{
		"s3": map[string]any{
			"region":                   "eu-west-1",
			"profile":                  "packages",
			"shared_config_files":      []any{"/etc/aws/config"},
			"shared_credentials_files": []any{"/etc/aws/credentials", "/run/aws/credentials"},
			"access_key":               "AKIAEXAMPLE",
			"secret_key":               "secret",
			"token":                    "token",
			"assume_role": map[string]any{
				"role_arn": "arn:aws:iam::123456789012:role/pkg-read", "session_name": "jamfplatform", "external_id": "ext",
			},
			"assume_role_with_web_identity": map[string]any{
				"role_arn": "arn:aws:iam::123456789012:role/pkg-oidc", "session_name": "ci", "web_identity_token": "jwt",
			},
			"endpoints":      map[string]any{"s3": "https://minio.internal:9000", "sts": "https://sts.internal"},
			"use_path_style": true,
		},
		"azure": map[string]any{
			"tenant_id": "tenant", "client_id": "client",
			"use_oidc": true, "oidc_token_file_path": "/run/token",
			"oidc_request_url": "https://issuer.example.com", "oidc_request_token": "request",
			"use_msi": false, "msi_endpoint": "http://169.254.169.254",
			"client_secret_file_path": "/run/secret",
			"environment":             "china",
		},
		"https": []any{
			map[string]any{"host": "artifactory.example.com", "bearer_token": "bearer"},
			map[string]any{"host": "files.example.com", "username": "ci", "password": "pw"},
			map[string]any{"host": "dl.example.com", "header": map[string]any{"name": "X-Api-Key", "value": "key"}},
		},
	}))
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v", unknown)
	}

	s3 := config.S3
	if s3.Region != "eu-west-1" || s3.Profile != "packages" || s3.AccessKey != "AKIAEXAMPLE" || s3.SecretKey != "secret" || s3.Token != "token" {
		t.Errorf("s3 scalars not read")
	}
	if !slices.Equal(s3.SharedConfigFiles, []string{"/etc/aws/config"}) || !slices.Equal(s3.SharedCredentialsFiles, []string{"/etc/aws/credentials", "/run/aws/credentials"}) {
		t.Errorf("s3 shared files not read")
	}
	if s3.AssumeRole == nil || s3.AssumeRole.RoleARN != "arn:aws:iam::123456789012:role/pkg-read" || s3.AssumeRole.SessionName != "jamfplatform" || s3.AssumeRole.ExternalID != "ext" {
		t.Errorf("s3 assume_role not read")
	}
	web := s3.AssumeRoleWithWebIdentity
	if web == nil || web.RoleARN != "arn:aws:iam::123456789012:role/pkg-oidc" || web.SessionName != "ci" || web.WebIdentityToken != "jwt" || web.WebIdentityTokenFile != "" {
		t.Errorf("s3 assume_role_with_web_identity not read")
	}
	if s3.EndpointS3 != "https://minio.internal:9000" || s3.EndpointSTS != "https://sts.internal" || !s3.UsePathStyle {
		t.Errorf("s3 endpoints not read")
	}

	azure := config.Azure
	if azure.TenantID != "tenant" || azure.ClientID != "client" || azure.OIDCTokenFile != "/run/token" || azure.OIDCRequestURL != "https://issuer.example.com" || azure.OIDCRequestToken != "request" {
		t.Errorf("azure oidc values not read")
	}
	if azure.UseOIDC == nil || !*azure.UseOIDC {
		t.Errorf("use_oidc = true must resolve to true")
	}
	if azure.UseMSI == nil || *azure.UseMSI {
		t.Errorf("use_msi = false must resolve to an explicit false")
	}
	if azure.UseCLI != nil {
		t.Errorf("an unset use_cli must stay nil so the default of true can apply")
	}
	if azure.MSIEndpoint != "http://169.254.169.254" || azure.ClientSecretFile != "/run/secret" || azure.Environment != "china" {
		t.Errorf("azure remaining values not read")
	}

	if len(config.HTTPS) != 3 {
		t.Fatalf("https entries = %d", len(config.HTTPS))
	}
	if h := config.HTTPS[0]; h.Host != "artifactory.example.com" || h.BearerToken != "bearer" {
		t.Errorf("https bearer entry not read")
	}
	if h := config.HTTPS[1]; h.Host != "files.example.com" || h.Username != "ci" || h.Password != "pw" {
		t.Errorf("https basic entry not read")
	}
	if h := config.HTTPS[2]; h.Host != "dl.example.com" || h.HeaderName != "X-Api-Key" || h.HeaderValue != "key" {
		t.Errorf("https header entry not read")
	}
}

func TestResolveFileSourcesRecordsUnknownValues(t *testing.T) {
	cases := []struct {
		name        string
		fileSources any
		want        []string
	}{
		{"whole block", unknownValue{}, []string{"file_sources"}},
		{"s3 block", map[string]any{"s3": unknownValue{}}, []string{"file_sources.s3"}},
		{"azure block", map[string]any{"azure": unknownValue{}}, []string{"file_sources.azure"}},
		{"https list", map[string]any{"https": unknownValue{}}, []string{"file_sources.https"}},
		{"s3 leaves", map[string]any{"s3": map[string]any{
			"region": unknownValue{}, "secret_key": unknownValue{}, "use_path_style": unknownValue{},
		}}, []string{"file_sources.s3.region", "file_sources.s3.secret_key", "file_sources.s3.use_path_style"}},
		{"s3 nested", map[string]any{"s3": map[string]any{
			"assume_role":                   map[string]any{"role_arn": unknownValue{}},
			"assume_role_with_web_identity": unknownValue{},
			"endpoints":                     map[string]any{"s3": unknownValue{}},
		}}, []string{"file_sources.s3.assume_role.role_arn", "file_sources.s3.assume_role_with_web_identity", "file_sources.s3.endpoints.s3"}},
		{"s3 list element", map[string]any{"s3": map[string]any{
			"shared_config_files": []any{"/etc/aws/config", unknownValue{}},
		}}, []string{"file_sources.s3.shared_config_files"}},
		{"azure leaves", map[string]any{"azure": map[string]any{
			"client_id": unknownValue{}, "use_cli": unknownValue{}, "client_secret": unknownValue{},
		}}, []string{"file_sources.azure.client_id", "file_sources.azure.client_secret", "file_sources.azure.use_cli"}},
		{"https entry", map[string]any{"https": []any{
			map[string]any{"host": "a.example.com", "bearer_token": "t"},
			unknownValue{},
		}}, []string{"file_sources.https[1]"}},
		{"https leaves", map[string]any{"https": []any{
			map[string]any{"host": unknownValue{}, "bearer_token": "t"},
			map[string]any{"host": "b.example.com", "header": map[string]any{"name": "X-Api-Key", "value": unknownValue{}}},
		}}, []string{"file_sources.https[0].host", "file_sources.https[1].header.value"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, unknown, diags := resolveFileSources(context.Background(), fileSourcesObject(t, tc.fileSources))
			if diags.HasError() {
				t.Fatalf("diagnostics: %v", diags)
			}
			slices.Sort(unknown)
			if !slices.Equal(unknown, tc.want) {
				t.Errorf("unknown paths:\n got %v\nwant %v", unknown, tc.want)
			}
		})
	}
}

func TestResolveFileSourcesDropsAnHTTPSEntryWithAnUnknownValue(t *testing.T) {
	config, _, diags := resolveFileSources(context.Background(), fileSourcesObject(t, map[string]any{"https": []any{
		map[string]any{"host": "a.example.com", "bearer_token": unknownValue{}},
		map[string]any{"host": "b.example.com", "bearer_token": "t"},
	}}))
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if len(config.HTTPS) != 1 || config.HTTPS[0].Host != "b.example.com" {
		t.Errorf("https = %d entries; a credential with an unknown secret must be left out whole", len(config.HTTPS))
	}
}
