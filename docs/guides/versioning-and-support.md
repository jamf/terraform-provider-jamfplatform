---
page_title: "Versioning, deprecations and supported versions"
description: |-
  How provider versions are numbered, how much notice a removal gets, and which Jamf Pro and Terraform versions the provider supports.
---

# Versioning, deprecations and supported versions

Read this before you pin or upgrade the provider. A minor release can break your configuration,
you get at least 90 days' notice before the provider removes an attribute, and a Jamf Pro tenant
older than the provider's build target still works, with a warning.

-> The registry publishes this page with each provider release, so the copy for an older release
can be out of date. The
[latest version](https://registry.terraform.io/providers/jamf/jamfplatform/latest/docs/guides/versioning-and-support)
holds the current policy.

## Provider versions

The provider numbers its releases `MAJOR.MINOR.PATCH` and departs from semantic versioning in one
way: a minor release can contain a breaking change.

| Release | Contains |
|---|---|
| Patch (`1.9.0` → `1.9.1`) | Bug fixes. The schema and the behaviour stay the same. |
| Minor (`1.9.0` → `1.10.0`) | New resources and attributes, deprecations, and breaking changes: a removed or renamed attribute, or a change in behaviour. |
| Major (`1.x` → `2.0.0`) | A coordinated clean-up release. A single breaking change ships in a minor release. |

Pin to a patch series, and move to each new minor version on purpose:

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
also accepts `1.10.0`, which can break your configuration.

The [release notes](https://github.com/jamf/terraform-provider-jamfplatform/releases) list the
changes in each release and call out the breaking ones. Read them before you move to a new minor
version.

Jamf releases the provider about once a week, on a best-effort basis, and ships a fix for a severe
bug sooner. Jamf guarantees no schedule.

## Deprecations

Before the provider removes or renames an attribute you can set, it deprecates that attribute. The
removal follows at least **90 days** after the release that shipped the deprecation.

During those 90 days:

- You can use the replacement from the same release, and the deprecated attribute keeps working.
- `terraform plan` and `terraform validate` print a warning that names the replacement and the
  earliest removal date. For example: ``Deprecated: use `script_assignments` instead, the same
  list without the wrapping block. The provider will not remove `scripts` before 2027-02-15.``
- The attribute's documentation on this registry carries the same notice.
- You can set the old attribute or the new one, and the provider rejects a configuration that
  sets both. An import, and configuration that Terraform generates for you, use the new one.

The date in the warning is the earliest the attribute can go. A minor release on or after that
date removes it.

That release migrates your state on the first plan. You have to change your configuration
yourself: if it still sets the removed attribute, the plan fails on an unsupported argument.

### Changes of type

One name cannot hold two types, so an attribute that changes type gets a new name and goes
through the deprecation above.

A set that becomes a list of the same elements is the exception. It changes in place, with no
deprecation, because the configuration you wrote stays valid. The release notes call it out:
expressions that depend on a set, such as `setunion` or an `==` comparison against a set, stop
working.

### When Jamf retires an API

Jamf deprecates and removes API endpoints on its own schedule. If Jamf provides a replacement
endpoint, the provider moves to it before Jamf removes the old one. Your configuration stays the
same unless the release notes say otherwise.

If Jamf retires an API with no replacement, the provider deprecates the resource or attribute that
depends on it and removes it in a later release. The release notes and the resource's documentation
say so. Jamf's removal date drives the timing, so you can get less than 90 days' notice.

## Supported Jamf Pro versions

Of the products the provider manages, Jamf Pro alone has a version per tenant. Jamf builds each
provider release against one Jamf Pro API version and tests it against the current Jamf Cloud
release. The [provider page](../index.md) lists the build version under "Supported Jamf products
and tenant version targets".

You can run the provider against an older tenant:

- **You get a warning on each run.** `terraform plan` and `terraform apply` report "Jamf Pro
  tenant older than provider build target" with both versions, and then carry on. A resource can
  still fail during apply if it uses an endpoint or field your version lacks.
- **A resource that needs a newer version stops the run.** The provider reports the version the
  resource needs and the version your tenant runs, before it plans anything.

If you cannot upgrade your tenant, pin the provider to the last release built against your Jamf
Pro version.

## Other Jamf products

Jamf deploys these products to every tenant as it releases them, so they have no tenant version
and the provider sets no minimum: Blueprints, Compliance Benchmarks, device groups, devices and
device actions, Jamf Security Cloud, Jamf AI Governance and Jamf Account. The provider follows
the current API for each.

## Supported Terraform versions

The provider needs Terraform 1.13.0 or later, or OpenTofu 1.6.0 or later.

## Upgrade guides

A release that needs more than a version bump gets its own guide:

- [Moving to the jamf namespace](namespace-migration), if you are coming from
  `jamf-concepts/jamfplatform`.
- [Upgrading to the Platform API GA](platform-api-ga), if you are coming from a pre-GA beta
  release.
