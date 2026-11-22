// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeHost(t *testing.T) {
	cases := []struct {
		host, want string
	}{
		{"files.example.com", "files.example.com:443"},
		{"FILES.Example.COM", "files.example.com:443"},
		{"files.example.com:443", "files.example.com:443"},
		{"files.example.com:8443", "files.example.com:8443"},
		{"files.example.com.", "files.example.com:443"},
		{"10.0.0.5", "10.0.0.5:443"},
		{"10.0.0.5:9000", "10.0.0.5:9000"},
		{"[2001:db8::1]", "[2001:db8::1]:443"},
		{"[2001:DB8::1]:8443", "[2001:db8::1]:8443"},
		{"[::0001]", "[::1]:443"},
		{"build_cache.internal", "build_cache.internal:443"},
		{"localhost", "localhost:443"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got, err := NormalizeHost(tc.host)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeHostRejects(t *testing.T) {
	for _, host := range []string{
		"",
		".",
		"https://files.example.com",
		"files.example.com/packages",
		"files.example.com?x=1",
		"user@files.example.com",
		"files.example.com ",
		"files.example.com:",
		"files.example.com:0",
		"files.example.com:65536",
		"files.example.com:https",
		"files.example.com:0443",
		"2001:db8::1",
		"[]",
		"[host",
		"host]",
		"[h]:80]",
		"[files.example.com]",
		"[fe80::1%eth0]",
		"[10.0.0.5]",
		"files.example.com\n",
		"files\texample.com",
		"*.example.com",
		"..",
		"exa%mple.com",
		"-files.example.com",
		"files..example.com",
		"f\u0130les.example.com",
		"\uff46iles.example.com",
		"files.example.com\u3002",
		"10.0.0.5.",
	} {
		t.Run(host, func(t *testing.T) {
			got, err := NormalizeHost(host)
			if _, ok := errors.AsType[*SyntaxError](err); !ok {
				t.Fatalf("want a SyntaxError, got %q and %v", got, err)
			}
		})
	}
}

func TestConfigurationNeverPrintsASecret(t *testing.T) {
	const secret = "s3cr3t-value"
	use := true
	config := Config{
		S3: S3Config{
			AccessKey: secret,
			SecretKey: secret,
			Token:     secret,
			AssumeRoleWithWebIdentity: &S3AssumeRoleWithWebIdentity{
				WebIdentityToken: secret,
			},
		},
		Azure: AzureConfig{
			UseOIDC:          &use,
			OIDCToken:        secret,
			OIDCRequestToken: secret,
			ClientSecret:     secret,
		},
		HTTPS: []HTTPSCredential{{Host: "files.example.com", BearerToken: secret, Password: secret, HeaderValue: secret}},
	}
	values := []any{
		config, &config,
		config.S3, &config.S3,
		config.S3.AssumeRoleWithWebIdentity, *config.S3.AssumeRoleWithWebIdentity,
		config.Azure, &config.Azure,
		config.HTTPS, config.HTTPS[0], &config.HTTPS[0],
		New(config, nil),
	}
	for _, value := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
			if out := fmt.Sprintf(verb, value); strings.Contains(out, secret) {
				t.Errorf("%T printed with %s leaks a secret: %s", value, verb, out)
			}
		}
	}
}
