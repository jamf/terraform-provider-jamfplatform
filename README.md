# terraform-provider-jamfplatform

> **📘 Start here — [Managing the Jamf Platform with Terraform: the Jamf Platform provider](https://developer.jamf.com/platform-api/docs/managing-the-jamf-platform-with-terraform-the-jamf-platform-provider)**
>
> The official getting-started guide on the Jamf Developer Portal. Covers installing Terraform, creating API credentials, configuring the provider, writing your first device groups / compliance benchmarks / blueprints, applying a configuration, and bringing an existing tenant under management. New to this provider or to Terraform? Read that first.

Provides resources and data sources for managing the products and services available through the [Jamf Platform API](https://developer.jamf.com/platform-api/):

* [Compliance Benchmark Engine](https://learn.jamf.com/en-US/bundle/jamf-compliance-benchmarks-configuration-guide/page/Compliance_Benchmarks_Configuration_Guide.html)
  * [API Reference](https://developer.jamf.com/platform-api/reference/getbenchmark)
* [Blueprints](https://learn.jamf.com/en-US/bundle/jamf-pro-blueprints-configuration-guide/page/Jamf_Pro_Blueprints_Configuration_Guide.html)
  * [API Reference](https://developer.jamf.com/platform-api/reference/listblueprints)
* Device Groups
  * [API Reference](https://developer.jamf.com/platform-api/reference/get_v1-tenant-tenantid-device-groups)
* [Devices](https://developer.jamf.com/platform-api/reference/get_v1-tenant-tenantid-devices)
* [Device Actions](https://developer.jamf.com/platform-api/reference/post_v1-tenant-tenantid-devices-id-check-in)

It additionally provides resources and data sources for these products, each under its own namespace:

* [Jamf Pro](https://developer.jamf.com/platform-api/reference/get_v1-tenant-tenantid-account-groups) — `jamfplatform_pro_*`
* [Jamf Security Cloud](https://learn.jamf.com/en-US/bundle/jamf-security-cloud-documentation/page/Jamf_Security_Cloud_Documentation.html) — `jamfplatform_security_cloud_*`: Custom DNS, ZTNA gateways and access policy, UEM Connect, device groups and activation profiles
* Jamf AI Governance — `jamfplatform_ai_governance_*`: the managed settings delivered to an AI tool, and the product catalogue
* Jamf Account — `jamfplatform_account_*`: organization-level single sign-on, meaning claimed domains and the identity providers that sign users in for them

Further Jamf products are expected to follow. See **Supported Jamf products and versions** below for the per-product version requirements, and the [guides](./docs/guides/) for the families that carry behaviour worth reading first.

## Acknowledgements

This provider builds on a path charted by [Deployment Theory](https://github.com/deploymenttheory) and their [`terraform-provider-jamfpro`](https://github.com/deploymenttheory/terraform-provider-jamfpro) — first released in early 2024, it has grown into the most comprehensive community Terraform provider for Jamf and the one the community rallies behind. It proved the depth of demand for managing Jamf as code and sets the bar for what a Jamf provider could be; this provider would not exist in its current form without that groundwork. `terraform-provider-jamfpro` remains an independent, actively maintained project, and we're grateful to its maintainers for the example they set.

## Upgrading from an earlier release

* Coming from `jamf-concepts/jamfplatform`: [Moving to the jamf namespace](docs/guides/namespace-migration.md).
* Coming from a pre-GA beta release: [Upgrading to the Platform API GA](docs/guides/platform-api-ga.md).

## Requirements

* Terraform >= 1.13.0, or OpenTofu >= 1.6.0

### Supported Jamf products and versions

| Product | Resource namespace | Tenant version |
|---------|--------------------|----------------|
| Jamf Pro | `jamfplatform_pro_*` | Built against the Jamf Pro API version in [`ProviderMinJamfProVersion`](./internal/providerdata/providerdata.go). An older tenant gets a warning on every plan and apply. |
| Jamf Platform Services (Blueprints, Device Groups, Devices, Device Actions, Compliance Benchmarks) | no product prefix, such as `jamfplatform_blueprints_blueprint` | Continuously deployed, no requirement. |
| Jamf Security Cloud (Custom DNS, ZTNA, UEM Connect, device groups, activation profiles) | `jamfplatform_security_cloud_*` | Continuously deployed, no requirement. A separate entitlement from Jamf Pro. |
| Jamf AI Governance | `jamfplatform_ai_governance_*` | Continuously deployed, no requirement. |
| Jamf Account (single sign-on) | `jamfplatform_account_*` | Continuously deployed, no requirement. Served from the US gateway alone. |

Which of these an API integration reaches depends on the scope it was created with; the [provider documentation](https://registry.terraform.io/providers/jamf/jamfplatform/latest/docs) says which scope each family needs.

### Versioning and deprecations

A minor release can contain a breaking change, so pin to a patch series (`~> 1.9.0`). The provider deprecates an attribute at least 90 days before it removes it. [Versioning, deprecations and supported versions](docs/guides/versioning-and-support.md) has the full policy and says what an older Jamf Pro tenant gets.

**Credentials are the same across all products**: a single OAuth client (`JAMFPLATFORM_CLIENT_ID` / `JAMFPLATFORM_CLIENT_SECRET`) scoped to the relevant API areas on your tenant.

## Using the Provider in your own Terraform Projects

The jamfplatform provider is published in the [Hashicorp](https://registry.terraform.io/providers/jamf/jamfplatform) and [OpenTofu](https://search.opentofu.org/provider/jamf/jamfplatform) registries.

For a step-by-step walkthrough — from installing Terraform through to applying your first configuration and importing an existing tenant — see the Jamf Developer Portal guide: [**Managing the Jamf Platform with Terraform: the Jamf Platform provider**](https://developer.jamf.com/platform-api/docs/managing-the-jamf-platform-with-terraform-the-jamf-platform-provider).

For provider block/variable reference, refer to the registry link above for your platform of choice.

---

## Provider Configuration Reference and Example Usage

Refer to the [documentation](https://registry.terraform.io/providers/jamf/jamfplatform/latest/docs) for a full list of resources and data sources, their usage and Terraform examples.

---

## Troubleshooting

### Debug logging

Terraform's log level is set with `TF_LOG`. At `DEBUG` the provider logs every HTTP request and response it makes to the Jamf Platform API — method, URL, status code, response headers, and the request and response bodies:

```shell
TF_LOG=DEBUG terraform apply
TF_LOG=DEBUG terraform apply 2> debug.log   # capture to a file
```

`TF_LOG=TRACE` adds Terraform core's own plan/apply internals on top. Use `TF_LOG_PROVIDER=DEBUG` to raise the level for providers only and leave Terraform core quieter.

### Debug logs and secrets

Debug logs are the fastest way to see what the provider actually sent, which means they are also the most likely place for a credential to end up. The provider therefore redacts before it logs:

- **Headers** — `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie` and `X-Api-Key` are logged as `REDACTED`.
- **Request and response bodies** — values of known credential-bearing fields are replaced with `REDACTED`. This covers plaintext passwords and their `_sha256` echoes, recovery and encryption keys, client secrets, bearer and enrolment tokens, keystore blobs, and secret values inside configuration profile payloads (including plist `<key>`/`<string>` pairs, which sit inside the `payloads` element).
- **Unparseable bodies** — a body that is neither JSON nor XML cannot be checked for credentials, so it is withheld from the log rather than written out raw.
- **File uploads** — multipart bodies are never rendered; the log records `<multipart body>`.

Redaction is matched against known field names, so treat it as a strong safety net rather than a guarantee. Two things follow from that:

- Review a debug log before you attach it to a support ticket, paste it into an issue, or leave it in CI output. CI logs in particular are often readable by more people than the person who triggered the run.
- If you find a credential in a debug log that should have been redacted, please report it — see [SECURITY.md](./SECURITY.md).

Redaction applies to what the provider logs. It does not change what Terraform itself writes: values marked sensitive are masked in Terraform's plan and apply output, but a `write-only` argument's value still exists in your configuration and in any variable file or environment variable that supplies it.

---

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](./CONTRIBUTING.md) and [TESTING.md](./TESTING.md) for the full workflow. In short, for changes that add or modify resources, data sources, list resources, actions, or functions:

1. **Add Go unit tests** — schema validation, input builders, state builders, and (where relevant) state upgraders.
2. **Add Go acceptance tests** — `resource_acceptance_test.go` (or `datasource_acceptance_test.go`) with the `//go:build acceptance` tag, using factories from `internal/testhelpers`. Run locally with `make testacc` against a test tenant.
3. **Update examples** — add `.tf` files under the appropriate `examples/` subdirectory.
4. **Run `make generate`** — regenerates `docs/` from schema descriptions and applies copyright headers.
5. **CI** — `.github/workflows/integration-tests.yml` runs build, lint, docs-generation check, and the Go unit suite on every PR. The Go acceptance suite runs against a real tenant after a reviewer approves the `acceptance` environment gate.

For bug reports, feature requests, or general discussion, please use [GitHub Issues](https://github.com/jamf/terraform-provider-jamfplatform/issues).

---

## Feedback & Discussion

Please contact the project principles via [GitHub Issues](https://github.com/jamf/terraform-provider-jamfplatform/issues).

The Jamf Terraform community has discussions in #terraform-provider-jamfpro on [MacAdmins Slack](https://www.macadmins.org/). This channel is primarily focused on discussion and community support relating to the [jamfpro](https://github.com/deploymenttheory/terraform-provider-jamfpro) provider that is owned and maintained by our friends, [Deployment Theory](https://github.com/deploymenttheory).

## Included components

The following third party acknowledgements and licenses are incorporated by reference:

* [Jamf Platform Go SDK](https://github.com/jamf/jamfplatform-go-sdk) ([MIT](https://github.com/jamf/jamfplatform-go-sdk?tab=MIT-1-ov-file))
* [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) ([MPL](https://github.com/hashicorp/terraform-plugin-framework?tab=MPL-2.0-1-ov-file))
* [Terraform Plugin Log](https://github.com/hashicorp/terraform-plugin-log) ([MPL](https://github.com/hashicorp/terraform-plugin-log?tab=MPL-2.0-1-ov-file))

&nbsp;

*Copyright 2026, Jamf Software LLC.*
