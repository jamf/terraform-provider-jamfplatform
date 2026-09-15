// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package smtp_server_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const smtpAddr = "jamfplatform_pro_smtp_server.test"

// checkSingletonRecordStillExists verifies the SMTP Server settings record
// persists on the tenant after Terraform destroys the resource from state. The
// remote Delete is a no-op, so the API must still return the record post-destroy.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "SMTP Server", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetSmtpServerV2(ctx)
	})
}

// TestAccResource_ProSmtpServer_Basic exercises the BASIC happy-path plus a
// multi-attribute Update (host/port/encryption/timeout/display_name/username all
// change), then an import round-trip. Throwaway host/credentials — the PUT does
// not validate SMTP connectivity (that is the separate test-email endpoint, out
// of scope).
func TestAccResource_ProSmtpServer_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						enabled             = true
						authentication_type = "BASIC"
						sender_settings = {
							email_address = "notifications@example.com"
							display_name  = "Example Notifications"
						}
						connection_settings = {
							host               = "192.0.2.25"
							port               = 465
							encryption_type    = "SSL"
							connection_timeout = 30
						}
						basic_auth_credentials = {
							username            = "svc@example.com"
							password            = "dummy-password-1"
							password_wo_version = 1
						}
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(smtpAddr, "id", "singleton"),
					resource.TestCheckResourceAttr(smtpAddr, "authentication_type", "BASIC"),
					resource.TestCheckResourceAttr(smtpAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(smtpAddr, "sender_settings.display_name", "Example Notifications"),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.host", "192.0.2.25"),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.port", "465"),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.encryption_type", "SSL"),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.connection_timeout", "30"),
					resource.TestCheckResourceAttr(smtpAddr, "basic_auth_credentials.username", "svc@example.com"),
				),
			},
			{
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						enabled             = false
						authentication_type = "BASIC"
						sender_settings = {
							email_address = "notifications@example.com"
							display_name  = "Renamed Sender"
						}
						connection_settings = {
							host               = "198.51.100.25"
							port               = 587
							encryption_type    = "TLS_1_2"
							connection_timeout = 60
						}
						basic_auth_credentials = {
							username            = "newsvc@example.com"
							password            = "dummy-password-1"
							password_wo_version = 1
						}
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(smtpAddr, "enabled", "false"),
					resource.TestCheckResourceAttr(smtpAddr, "sender_settings.display_name", "Renamed Sender"),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.host", "198.51.100.25"),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.port", "587"),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.encryption_type", "TLS_1_2"),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.connection_timeout", "60"),
					resource.TestCheckResourceAttr(smtpAddr, "basic_auth_credentials.username", "newsvc@example.com"),
				),
			},
			{
				ResourceName:      smtpAddr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
				// WriteOnly secret + its rotation trigger are not recoverable on
				// import (never returned by the server / no prior state); timeouts
				// are import-synthesised.
				ImportStateVerifyIgnore: []string{
					"timeouts",
					"basic_auth_credentials.password",
					"basic_auth_credentials.password_wo_version",
				},
			},
		},
	})
}

// TestAccResource_ProSmtpServer_None is the NONE-mode happy-path: connection
// settings only, no credentials.
func TestAccResource_ProSmtpServer_None(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						authentication_type = "NONE"
						sender_settings = {
							email_address = "notifications@example.com"
						}
						connection_settings = {
							host            = "192.0.2.25"
							port            = 25
							encryption_type = "NONE"
						}
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(smtpAddr, "authentication_type", "NONE"),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.port", "25"),
					// connection_timeout defaults to 30.
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.connection_timeout", "30"),
					// Foreign credential blocks must be absent.
					resource.TestCheckNoResourceAttr(smtpAddr, "basic_auth_credentials.username"),
				),
			},
		},
	})
}

// TestAccResource_ProSmtpServer_UnconfiguredTenant covers the state a tenant that
// has never set up mail reads back in, and the two things that state has to
// support.
//
// The sender address and display name are both empty, which Jamf Pro accepts and
// round-trips while the connection is disabled (wire-probed 2026-09-05; enabling
// refuses each independently). Until issue #379 this resource carried a
// minimum-length validator on the address, so the tenant could not be declared at
// all and configuration generated from it could not be planned — the state is
// unreachable from every other test here, because a test that applies a
// configuration and reads it back can only produce a state the configuration
// already described.
//
// Step 2 is the generation guard, and step 3 asserts the replacement rule: the
// prohibition on an empty sender belongs to an ENABLED connection, and now fires
// at plan time instead of arriving as a 400 mid-apply.
//
// connection_settings.host is empty here too, and for the same reason: an
// unconfigured tenant reads back with an empty host alongside the empty sender
// address (one read returned authenticationType NONE, enabled false,
// senderSettings.emailAddress "" and connectionSettings.host "" together), a
// disabled write carrying an empty host round-trips, and an enabled one is refused
// 400 [FIELD_REQUIRED_FOR_SMTP]. So all three empty fields belong to the shape
// this test declares, and step 4 pins the host's half of the enable-time rule.
func TestAccResource_ProSmtpServer_UnconfiguredTenant(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const unconfigured = `
		resource "jamfplatform_pro_smtp_server" "test" {
			authentication_type = "NONE"
			enabled             = false
			sender_settings = {
				email_address = ""
				display_name  = ""
			}
			connection_settings = {
				host            = ""
				port            = 25
				encryption_type = "NONE"
			}
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: unconfigured,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(smtpAddr, "enabled", "false"),
					resource.TestCheckResourceAttr(smtpAddr, "sender_settings.email_address", ""),
					resource.TestCheckResourceAttr(smtpAddr, "sender_settings.display_name", ""),
					resource.TestCheckResourceAttr(smtpAddr, "connection_settings.host", ""),
				),
			},
			testhelpers.GenerateConfigStep(smtpAddr),
			{
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						authentication_type = "NONE"
						enabled             = true
						sender_settings = {
							email_address = ""
							display_name  = ""
						}
						connection_settings = {
							host            = "192.0.2.25"
							port            = 25
							encryption_type = "NONE"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`Sender email address required`),
			},
			{
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						authentication_type = "NONE"
						enabled             = true
						sender_settings = {
							email_address = "notifications@example.com"
							display_name  = "Jamf Pro"
						}
						connection_settings = {
							host            = ""
							port            = 25
							encryption_type = "NONE"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`SMTP server address required`),
			},
		},
	})
}

// TestAccResource_ProSmtpServer_GraphApi is the GRAPH_API happy-path: no
// connection settings, Graph credentials only.
func TestAccResource_ProSmtpServer_GraphApi(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						authentication_type = "GRAPH_API"
						sender_settings = {
							email_address = "notifications@example.com"
						}
						graph_api_credentials = {
							tenant_id                = "00000000-0000-0000-0000-000000000000"
							client_id                = "11111111-1111-1111-1111-111111111111"
							client_secret            = "dummy-graph-secret"
							client_secret_wo_version = 1
						}
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(smtpAddr, "authentication_type", "GRAPH_API"),
					resource.TestCheckResourceAttr(smtpAddr, "graph_api_credentials.tenant_id", "00000000-0000-0000-0000-000000000000"),
					resource.TestCheckResourceAttr(smtpAddr, "graph_api_credentials.client_id", "11111111-1111-1111-1111-111111111111"),
					// connection_settings must be absent in GRAPH_API mode.
					resource.TestCheckNoResourceAttr(smtpAddr, "connection_settings.host"),
				),
			},
		},
	})
}

// TestAccResource_ProSmtpServer_GoogleMail is the GOOGLE_MAIL happy-path. Linking
// a Google sender account requires an interactive Google OAuth grant performed in
// the Jamf Pro admin UI, which Terraform cannot drive; the apply may also require
// at least one granted account on some tenants. Opt in with
// JAMFPLATFORM_ACC_PRO_SMTP_GOOGLE=1 once the tenant has Google Auth configured.
func TestAccResource_ProSmtpServer_GoogleMail(t *testing.T) {
	if testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_SMTP_GOOGLE") == "" {
		t.Skip("set JAMFPLATFORM_ACC_PRO_SMTP_GOOGLE=1 to run GOOGLE_MAIL acceptance (requires out-of-band Google OAuth grant on the tenant)")
	}
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						authentication_type = "GOOGLE_MAIL"
						sender_settings = {
							email_address = "notifications@example.com"
						}
						google_mail_credentials = {
							client_id                = "google-client-id.apps.googleusercontent.com"
							client_secret            = "dummy-google-secret"
							client_secret_wo_version = 1
						}
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(smtpAddr, "authentication_type", "GOOGLE_MAIL"),
					resource.TestCheckResourceAttr(smtpAddr, "google_mail_credentials.client_id", "google-client-id.apps.googleusercontent.com"),
					// authentications is Computed and read-only.
					resource.TestCheckResourceAttrSet(smtpAddr, "google_mail_credentials.authentications.#"),
				),
			},
		},
	})
}

// TestAccResource_ProSmtpServer_ValidatorErrors exercises the discriminator
// ConfigValidator (missing required block, forbidden foreign block) and the
// secret↔wo_version AlsoRequires pairing.
func TestAccResource_ProSmtpServer_ValidatorErrors(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// BASIC without basic_auth_credentials.
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						authentication_type = "BASIC"
						sender_settings = { email_address = "n@example.com" }
						connection_settings = {
							host            = "192.0.2.25"
							port            = 465
							encryption_type = "SSL"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`basic_auth_credentials block required`),
			},
			{
				// GRAPH_API with a forbidden connection_settings block.
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						authentication_type = "GRAPH_API"
						sender_settings = { email_address = "n@example.com" }
						connection_settings = {
							host            = "192.0.2.25"
							port            = 465
							encryption_type = "SSL"
						}
						graph_api_credentials = {
							tenant_id     = "t"
							client_id     = "c"
							client_secret = "s"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`connection_settings block forbidden`),
			},
			{
				// Secret supplied without its rotation trigger (AlsoRequires).
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						authentication_type = "BASIC"
						sender_settings = { email_address = "n@example.com" }
						connection_settings = {
							host            = "192.0.2.25"
							port            = 465
							encryption_type = "SSL"
						}
						basic_auth_credentials = {
							username = "svc@example.com"
							password = "p"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`password_wo_version`),
			},
		},
	})
}

// TestAccResource_ProSmtpServer_RejectsNonSingletonImport verifies the import
// guard rejects any id other than "singleton".
func TestAccResource_ProSmtpServer_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_smtp_server" "test" {
						authentication_type = "NONE"
						sender_settings = { email_address = "n@example.com" }
						connection_settings = {
							host            = "192.0.2.25"
							port            = 25
							encryption_type = "NONE"
						}
					}
				`,
			},
			{
				ResourceName:  smtpAddr,
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProSmtpServer_OmitPreservesDisplayName proves the full-replace
// omit=preserve contract: an out-of-band display_name edit survives an apply that
// omits display_name, because the Optional+Computed attribute carries the prior
// value forward via UseStateForUnknown.
func TestAccResource_ProSmtpServer_OmitPreservesDisplayName(t *testing.T) {
	testhelpers.AccPreCheck(t)

	// Edit display_name out of band AFTER the first apply, then re-apply a config
	// that omits display_name and assert the out-of-band value is preserved.
	editDisplayNameOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetSmtpServerV2(context.Background())
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		dn := "Out-Of-Band Name"
		got.SenderSettings.DisplayName = &dn
		if _, err := c.UpdateSmtpServerV2(context.Background(), got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	configNoDisplayName := `
		resource "jamfplatform_pro_smtp_server" "test" {
			authentication_type = "NONE"
			sender_settings = { email_address = "notifications@example.com" }
			connection_settings = {
				host            = ""
				port            = 25
				encryption_type = "NONE"
			}
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: configNoDisplayName,
			},
			{
				PreConfig: editDisplayNameOutOfBand,
				Config:    configNoDisplayName,
				Check:     resource.TestCheckResourceAttr(smtpAddr, "sender_settings.display_name", "Out-Of-Band Name"),
			},
		},
	})
}
