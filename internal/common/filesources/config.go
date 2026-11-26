// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// redacted is what every configuration type prints as. The types hold access
// keys, client secrets and tokens, so none of them may reach a log line or an
// error through a %v.
const redacted = "<redacted>"

// Config is the provider's file_sources block as plain values: what was
// written in configuration and nothing else. A driver layers the store's own
// environment variables and default credential chain underneath it, so an
// empty Config is the ambient case, not an unconfigured one.
//
// UserAgent is the product token the provider identifies itself with to a
// store, as "name/version".
//
// HTTPTransport carries authenticated https downloads and Azure Blob Storage
// reads. Nil means
// http.DefaultTransport, which reads the proxy from the environment and
// trusts the system roots; the provider never sets it. It exists so a test
// can reach a local TLS server.
type Config struct {
	S3            S3Config
	Azure         AzureConfig
	HTTPS         []HTTPSCredential
	UserAgent     string
	HTTPTransport http.RoundTripper
}

// String keeps a Config out of logs.
func (Config) String() string { return redacted }

// GoString keeps a Config out of %#v output.
func (Config) GoString() string { return redacted }

// S3Config mirrors the arguments of Terraform's s3 backend that this provider
// supports.
type S3Config struct {
	Region                 string
	Profile                string
	SharedConfigFiles      []string
	SharedCredentialsFiles []string

	AccessKey string
	SecretKey string
	Token     string

	AssumeRole                *S3AssumeRole
	AssumeRoleWithWebIdentity *S3AssumeRoleWithWebIdentity

	EndpointS3   string
	EndpointSTS  string
	UsePathStyle bool
}

// String keeps an S3Config out of logs.
func (S3Config) String() string { return redacted }

// GoString keeps an S3Config out of %#v output.
func (S3Config) GoString() string { return redacted }

// S3AssumeRole is an IAM role to assume with the base credentials.
type S3AssumeRole struct {
	RoleARN     string
	SessionName string
	ExternalID  string
}

// S3AssumeRoleWithWebIdentity is an IAM role to assume with an OpenID Connect
// token, given inline or as a file to read.
type S3AssumeRoleWithWebIdentity struct {
	RoleARN              string
	SessionName          string
	WebIdentityToken     string
	WebIdentityTokenFile string
}

// String keeps the token out of logs.
func (S3AssumeRoleWithWebIdentity) String() string { return redacted }

// GoString keeps the token out of %#v output.
func (S3AssumeRoleWithWebIdentity) GoString() string { return redacted }

// AzureConfig mirrors the Microsoft Entra ID arguments of Terraform's azurerm
// backend that this provider supports.
//
// The three method switches are pointers because the backend reads each from
// an environment variable when the argument is absent, and use_cli defaults to
// true: nil means "not written in configuration", which is not the same as
// false.
type AzureConfig struct {
	TenantID string
	ClientID string

	UseOIDC          *bool
	OIDCToken        string
	OIDCTokenFile    string
	OIDCRequestURL   string
	OIDCRequestToken string

	UseMSI      *bool
	MSIEndpoint string

	UseCLI *bool

	ClientSecret     string
	ClientSecretFile string

	Environment string
}

// String keeps an AzureConfig out of logs.
func (AzureConfig) String() string { return redacted }

// GoString keeps an AzureConfig out of %#v output.
func (AzureConfig) GoString() string { return redacted }

// The Azure cloud names file_sources.azure.environment accepts.
const (
	AzureEnvironmentPublic       = "public"
	AzureEnvironmentChina        = "china"
	AzureEnvironmentUSGovernment = "usgovernment"
)

// AzureEnvironments lists the accepted environment names, for the schema
// validator and the description built from it.
func AzureEnvironments() []string {
	return []string{AzureEnvironmentPublic, AzureEnvironmentChina, AzureEnvironmentUSGovernment}
}

// HTTPSCredential is the credential for one https server. Exactly one of
// BearerToken, Username with Password, or HeaderName with HeaderValue is set;
// the provider schema enforces that.
type HTTPSCredential struct {
	Host string

	BearerToken string
	Username    string
	Password    string
	HeaderName  string
	HeaderValue string
}

// String keeps an HTTPSCredential out of logs.
func (HTTPSCredential) String() string { return redacted }

// GoString keeps an HTTPSCredential out of %#v output.
func (HTTPSCredential) GoString() string { return redacted }

// defaultHTTPSPort is the port a host with none stated is matched on.
const defaultHTTPSPort = "443"

// NormalizeHost turns the host of an https credential into the key it is
// matched on: the lower-cased hostname and the effective port, joined as
// "host:port". A host with no port gets 443, so "files.example.com" and
// "files.example.com:443" are one host, and a trailing dot is dropped so the
// fully-qualified spelling matches too. An IP address is rewritten in its
// canonical form, so two spellings of one IPv6 address are one host. A host
// holding a non-ASCII character is refused: lower-casing it is not the
// conversion a connection applies, so the key could name one host and the
// connection reach another.
//
// The error is a *SyntaxError. It refuses anything that is not a bare host: a scheme, a path, a query,
// user information, whitespace or a wildcard. A credential is matched on the host alone,
// so a value that names more than a host would promise a narrower scope than
// the one applied.
func NormalizeHost(host string) (string, error) {
	if host == "" {
		return "", syntaxErrorf("The host is empty.")
	}
	if strings.Contains(host, "://") {
		return "", syntaxErrorf("Give the host alone, without a scheme: a credential is only ever sent over https.")
	}
	for i := range len(host) {
		if host[i] >= 0x80 {
			return "", syntaxErrorf("Write the host in ASCII. An internationalised name goes in its xn-- form, the one a certificate carries.")
		}
	}
	if strings.ContainsAny(host, "/?#@") {
		return "", syntaxErrorf("Give the host alone, with an optional port and nothing else: no path, query or user name.")
	}

	name, port := host, defaultHTTPSPort
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		name = host
	} else if strings.Contains(host, ":") {
		n, p, err := net.SplitHostPort(host)
		if err != nil {
			return "", syntaxErrorf("The host is not a hostname with an optional port. Write an IPv6 address in brackets.")
		}
		name, port = n, p
		if strings.HasPrefix(host, "[") {
			name = "[" + n + "]"
		}
	}

	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
		return "", syntaxErrorf("The port is not a number from 1 to 65535.")
	}

	if after, ok := strings.CutPrefix(name, "["); ok {
		address, err := netip.ParseAddr(strings.TrimSuffix(after, "]"))
		if err != nil || !address.Is6() || address.Zone() != "" {
			return "", syntaxErrorf("The brackets do not hold an IPv6 address.")
		}
		return net.JoinHostPort(address.String(), port), nil
	}
	name = strings.ToLower(name)
	if address, err := netip.ParseAddr(name); err == nil && address.Is4() {
		return net.JoinHostPort(address.String(), port), nil
	}
	name = strings.TrimSuffix(name, ".")
	if _, err := netip.ParseAddr(name); err == nil {
		return "", syntaxErrorf("Write the IP address without a trailing dot.")
	}
	if len(name) > 253 || !hostnamePattern.MatchString(name) {
		return "", syntaxErrorf("The host is not a hostname or an IP address. A credential belongs to one host, so a wildcard is not read.")
	}
	return net.JoinHostPort(name, port), nil
}

// hostnamePattern is a DNS name: labels of letters, digits and inner hyphens.
// An underscore is allowed because internal names carry one often enough to
// matter, and refusing a name that resolves would be worse than passing it.
var hostnamePattern = regexp.MustCompile(`^[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?(\.[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?)*$`)
