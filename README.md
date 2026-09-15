# terraform-provider-jamfplatform

> **⚠️ This provider now publishes as `jamf/jamfplatform`.** Earlier releases carried `jamf-concepts/jamfplatform`, and Terraform records the namespace in state, so upgrading from one of them needs `terraform state replace-provider jamf-concepts/jamfplatform jamf/jamfplatform` in every workspace and state file before `terraform init -upgrade` will succeed. See [Moving to the jamf namespace](docs/guides/namespace-migration.md).

> **📘 Start here — [Managing the Jamf Platform with Terraform: the Jamf Platform provider](https://concepts.jamf.com/en/guides/infrastructure-as-code/managing-the-jamf-platform-with-terraform-the-jamf-platform-provider/)**
>
> The official getting-started guide on Jamf Concepts. Covers installing Terraform, creating API credentials, configuring the provider, writing your first device groups / compliance benchmarks / blueprints, applying a configuration, and bringing an existing tenant under management. New to this provider or to Terraform? Read that first.

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

Further Jamf products are expected to follow. See the **Supported Jamf products** section below for the per-product tenant version targets, and the [guides](./docs/guides/) for the families that carry behaviour worth reading first.

The Jamf Platform API reached general availability on 3 September 2026. Upgrading a configuration written against the public beta takes a coordinated change to the gateway host, the credentials and the scope attribute — see the [Upgrading to the Platform API GA](docs/guides/platform-api-ga.md) guide.

## Acknowledgements

This provider builds on a path charted by [Deployment Theory](https://github.com/deploymenttheory) and their [`terraform-provider-jamfpro`](https://github.com/deploymenttheory/terraform-provider-jamfpro) — first released in early 2024, it has grown into the most comprehensive community Terraform provider for Jamf and the one the community rallies behind. It proved the depth of demand for managing Jamf as code and sets the bar for what a Jamf provider could be; this provider would not exist in its current form without that groundwork. `terraform-provider-jamfpro` remains an independent, actively maintained project, and we're grateful to its maintainers for the example they set.

## Requirements

* Terraform >= 1.13.0, or OpenTofu >= 1.6.0

### Supported Jamf products and tenant version targets

The provider groups resources by the Jamf product they target. Some products are versioned at the customer-tenant level (the API spec the provider was generated from may be newer than what a given tenant is running); others are continuously-deployed Jamf Platform microservices with no tenant version concept.

| Product | Resource namespace | Built against API as of | Notes |
|---------|--------------------|--------------------------|-------|
| Jamf Pro | `jamfplatform_pro_*` | **11.31.0** (see [`ProviderMinJamfProVersion`](./internal/providerdata/providerdata.go) for the current source-tree value) | Tenants below this version emit an advisory warning at apply time. Individual resources and actions that depend on newer endpoints declare their own `minJamfProVersion` and hard-fail Configure on unsupported tenants — for example `jamfplatform_pro_service_discovery_enrollment` requires 11.25.0. |
| Jamf Platform Services (Blueprints, Device Groups, Devices, Device Actions, Compliance Benchmarks) | resources without a product-name prefix (e.g. `jamfplatform_blueprints_blueprint`, `jamfplatform_device_group`) | continuously-deployed | No tenant version requirement. No version fetch is performed against tenants that use only these resources. Blueprints and compliance benchmarks are reachable only by an integration scoped to a platform environment; a tenant-scoped one is refused when the resource is configured. The device and device-group constructs still work under either scope. |
| Jamf Security Cloud (Custom DNS, ZTNA, UEM Connect, device groups, activation profiles) | `jamfplatform_security_cloud_*` | continuously-deployed | No tenant version requirement. Security Cloud is a separate entitlement: a tenant can hold Jamf Pro without holding it, and these resources then fail with a named diagnostic saying the tenant is not entitled. |
| Jamf AI Governance | `jamfplatform_ai_governance_*` | continuously-deployed | No tenant version requirement. Reachable only by an integration scoped to a platform environment; a tenant-scoped one is refused when the resource is configured. |
| Jamf Account (single sign-on) | `jamfplatform_account_*` | continuously-deployed | No tenant version requirement. Reachable only by an integration scoped to organization management, which is also the only scope that reaches it. Served from the US gateway alone. |

Further Jamf products are expected to be added; each will get its own row, namespace, and version constant.

**Credentials are the same across all products**: a single OAuth client (`JAMFPLATFORM_CLIENT_ID` / `JAMFPLATFORM_CLIENT_SECRET`) scoped to the relevant API areas on your tenant.

## Using the Provider in your own Terraform Projects

The jamfplatform provider is published in the [Hashicorp](https://registry.terraform.io/providers/jamf/jamfplatform) and [OpenTofu](https://search.opentofu.org/provider/jamf/jamfplatform) registries.

For a step-by-step walkthrough — from installing Terraform through to applying your first configuration and importing an existing tenant — see the Jamf Concepts guide: [**Managing the Jamf Platform with Terraform: the Jamf Platform provider**](https://concepts.jamf.com/en/guides/infrastructure-as-code/managing-the-jamf-platform-with-terraform-the-jamf-platform-provider/).

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
