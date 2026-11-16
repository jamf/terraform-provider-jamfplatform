---
page_title: "Versioning, deprecations and supported versions"
description: |-
  How provider versions are numbered, how much notice a removal gets, and which Jamf Pro and Terraform versions the provider supports.
---

# Versioning, deprecations and supported versions

This guide covers three things you need before you pin or upgrade the provider: what a version
number promises, how much notice you get before something is removed, and which Jamf Pro and
Terraform versions the provider works with.

-> This page is published with each provider release, so an older copy can be out of date. The
[latest version](https://registry.terraform.io/providers/jamf/jamfplatform/latest/docs/guides/versioning-and-support)
is the current policy.

## Provider versions

Versions are numbered `MAJOR.MINOR.PATCH`. The provider does **not** follow strict semantic
versioning: a minor release can contain a breaking change.

| Release | Contains |
|---|---|
| Patch (`1.9.0` → `1.9.1`) | Bug fixes only. No schema change and no change in behaviour. |
| Minor (`1.9.0` → `1.10.0`) | Everything else: new resources and attributes, deprecations, and breaking changes such as a removed or renamed attribute or a change in behaviour. |
| Major (`1.x` → `2.0.0`) | Reserved for a coordinated clean-up release. A single breaking change does not wait for one. |

Because a minor release can break a configuration, pin to a patch series and move between minor
versions deliberately:

```terraform
terraform {
  required_providers {
    jamfplatform = {
      source  = "jamf/jamfplatform"
      version = "~> 1.9.0"
    }
  }
}
```

`~> 1.9.0` accepts `1.9.1` and later patches and stops before `1.10.0`. The more common `~> 1.9`
accepts `1.10.0` as well, which this provider does not treat as safe.

Every release lists its changes in the
[release notes](https://github.com/jamf/terraform-provider-jamfplatform/releases), and a breaking
change is called out there. Read them before you move to a new minor version. There is no fixed
release schedule.

## Deprecations

An attribute you can set is not removed or renamed without notice. The provider deprecates it
first, and the removal follows no sooner than **90 days** after the release that shipped the
deprecation.

During that window:

- The replacement is available from the same release, and the deprecated attribute keeps working.
- `terraform plan` and `terraform validate` print a warning that names the replacement and the
  earliest removal date, for example: ``Deprecated: use `script_assignments` instead, the same
  list without the wrapping block. The provider will not remove `scripts` before 2027-02-15.``
- The attribute's documentation on this registry carries the same notice.
- You can set the old attribute or the new one, not both. An import, and configuration that
  Terraform generates for you, use the new one.

The date is the earliest the attribute can go, not a scheduled removal. It is removed in a minor
release on or after that date.

When the removal ships, existing state is migrated for you on the first plan with the new
version. Your configuration is not: it must already use the replacement, or the plan fails on an
unsupported argument.

### Changes of type

An attribute that changes type gets a new name and goes through the deprecation above, since one
name cannot hold two types. There is one exception: a set that becomes a list of the same
elements changes in place, with no deprecation, because the configuration you wrote stays valid.
The release notes call it out, since expressions that depend on a set, such as `setunion` or an
`==` comparison against a set, stop working.

### When Jamf retires an API

Jamf deprecates and removes API endpoints on its own schedule. Where a replacement endpoint
exists, the provider moves to it before the old one is removed, and nothing changes in your
configuration unless the release notes say so.

Where Jamf retires an API with no replacement, the resource or attribute that depends on it is
deprecated and later removed, and the release notes and its documentation say so. A removal of
this kind is driven by Jamf's date, so the 90 days cannot always be honoured.

## Supported Jamf Pro versions

Jamf Pro is the only product the provider manages that has a version per tenant. Each provider
release is built against one Jamf Pro API version, listed under "Supported Jamf products and
tenant version targets" on the [provider page](../index.md), and is tested against the current
Jamf Cloud release.

A tenant on an older version still works, with two things to know:

- **A warning on every run.** `terraform plan` and `terraform apply` report "Jamf Pro tenant older
  than provider build target" with both versions. Nothing is blocked. A resource can still fail
  during apply if it uses an endpoint or field your version does not have.
- **A resource that needs a newer version refuses to run.** Where a resource cannot work below a
  given Jamf Pro version, the provider stops with an error naming the version it needs and the
  version your tenant reports, before anything is planned.

If your tenant cannot be upgraded, pin the provider to the last release built against your Jamf
Pro version.

## Other Jamf products

Blueprints, Compliance Benchmarks, device groups, devices and device actions, Jamf Security Cloud,
Jamf AI Governance and Jamf Account are deployed continuously by Jamf and have no version at the
tenant. There is no minimum to meet, and the provider follows the current API.

## Supported Terraform versions

The provider needs Terraform 1.13.0 or later, or OpenTofu 1.6.0 or later.

## Upgrade guides

A release that needs more than a version bump gets its own guide:

- [Moving to the jamf namespace](namespace-migration), if you are coming from
  `jamf-concepts/jamfplatform`.
- [Upgrading to the Platform API GA](platform-api-ga), if you are coming from a pre-GA beta
  release.
