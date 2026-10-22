---
page_title: "Upgrading to the Platform API GA"
description: |-
  What changed when the Jamf Platform API left beta: new credentials, a new base URL, environment scope, and the constructs that were removed.
---

# Upgrading to the Platform API GA

The Jamf Platform API reached general availability on 3 September 2026, and `v0.29.0` of this
provider is built against it. This guide covers upgrading a configuration written against the
public beta.

**Already on `v0.29.0` or later? None of this applies.** Read
[Moving to the jamf namespace](namespace-migration) instead, which is the only step between a
GA release and the current one.

Coming from `v0.28.1` or earlier, you need both guides. This one changes the gateway host, the
credentials and the scope attribute; the other rewrites the provider address recorded in state.
Do the state rewrite first, since it needs no network access and no provider binary, then work
through the sections here.

**Action is needed in every configuration built against the public beta.** Nothing carries over
untouched: the gateway host, the credentials and the scope attribute all change, and several
constructs have been removed. The beta gateway stopped serving the Platform API,
and beta credentials stopped working, so a configuration on `v0.28.1` or earlier cannot reach
it.

Each section below states the action required, or says explicitly that none is.

## Acknowledgements

Thank you to everyone who used this provider during the Platform API public beta. Bug reports and
feedback from beta participants shaped a substantial part of what ships at GA, including the
environment scope, proxy support and a range of resource behaviour. Continued feedback is welcome.

## Installing this release

The GA release was `v0.29.0`. It first published as `jamf-concepts/jamfplatform` and is available
under `jamf/jamfplatform` as well, which is where a configuration coming from the beta today
should point:

```hcl
terraform {
  required_providers {
    jamfplatform = {
      source  = "jamf/jamfplatform"
      version = ">= 0.29.0"
    }
  }
}
```

The namespace is recorded in state, so this needs `terraform state replace-provider` as well as
`terraform init -upgrade`. See [Moving to the jamf namespace](namespace-migration) for the order
to run them in. A configuration pinned to a `0.29.0-rc.*` release candidate is superseded either
way: the last candidate differs from the GA release.

Upgrade the provider, change `base_url` and replace the credentials as one change, in one
workspace, and verify a plan before moving on. The three are coupled — see
[Action needed](#action-needed).

## Action needed

Per workspace, and all of it in one change: the provider version, the host and the credentials are
coupled, and no partial combination works. A workspace on a `v0.29.0` release candidate is already
on the GA host, and every other step below still applies to it.

1. **Take a state backup.** It depends on nothing else, and step 5 edits state. A local backend
   writes its own backup as it goes; a remote backend does not, so this is the only copy you will
   have to fall back on.
2. **Register a replacement API integration and update the provider credentials.** Beta
   credentials are revoked, and a beta client cannot be migrated. Record the permissions each beta
   integration held beforehand, so the equivalents can be selected. See
   [API integration credentials](#api-integration-credentials).
3. **Register it as environment-scoped, and replace `tenant_id` with `environment_id`.** Choose
   tenant scope only if single-product access is deliberately what you want: it is the legacy
   option, costs one integration per tenant per product, and cannot hold the blueprint or
   compliance-benchmark permissions at all. See [Scope](#scope).
4. **Set `base_url` to the GA gateway.** The beta host stopped serving the API. See [Base URL](#base-url).
5. **Remove the deleted resources from state.** After the upgrade, a workspace still holding one of
   them cannot produce a plan of any kind. See [Removed constructs](#removed-constructs).
6. **Extend the integration's permissions where a workspace uses
   `jamfplatform_pro_patch_software_title`.** Its data source, list resource and `terraform import`
   now read the tenant's patch source catalogues, so two read permissions must be added. See
   [Attribute removals and deprecations](#attribute-removals-and-deprecations).

## Removed constructs

Several endpoints were unpublished at GA. The provider cannot call an endpoint that is gone, so the
constructs built on them have been removed.

### Managed resources (state edit required)

| Removed resource | Reason | Replacement |
|---|---|---|
| `jamfplatform_pro_api_client` | `/v1/api-integrations` unpublished, for security hardening | Jamf Account, Platform API integrations UI |
| `jamfplatform_pro_api_role` | `/v1/api-roles` unpublished, for security hardening | Jamf Account, Platform API integrations UI |

The API client and API role endpoints were unpublished to close a privilege-escalation path: an
API integration was able to create a client or role holding privileges the creating integration
did not itself hold.

**Action needed: remove the state entries.** Both shipped in `v0.28.1`, so existing state
files contain them. Terraform requires a schema
for every resource type present in state before it can complete `plan`, `apply` or `destroy`. Once
the provider no longer implements the type, every operation in that workspace fails, including
operations unrelated to these resources. Deleting the `resource` block is not enough: the state
entry remains. The error looks like this:

```
Error: no schema available for jamfplatform_pro_api_client.example while reading state;
this is a bug in Terraform and should be reported
```

Despite the wording, this is not a Terraform defect. Remove the state entries with
[`terraform state rm`](https://developer.hashicorp.com/terraform/cli/commands/state/rm). It is a
state-only operation, needs no schema, and works both before and after the upgrade:

```sh
# identify the affected addresses
terraform state list | grep -E 'jamfplatform_pro_api_(client|role)'

# preview the removal without modifying state
terraform state rm -dry-run 'jamfplatform_pro_api_client.example'

# remove each address reported by the list command
terraform state rm 'jamfplatform_pro_api_client.example'
```

A single address removes every instance of that resource, including instances created by `count`
or `for_each`. A local backend writes a `terraform.tfstate.<timestamp>.backup` file first. On a
remote backend, take an equivalent backup yourself.

`terraform state rm` only ends Terraform's management of the object. The API client or role itself
remains in place in Jamf Pro.

### Data sources and list resources (configuration edit only)

**Action needed: delete the blocks.** These hold no state, so removing them from the
configuration is the whole of it.

Data sources: `jamfplatform_pro_api_client`, `jamfplatform_pro_api_clients`,
`jamfplatform_pro_api_role`, `jamfplatform_pro_api_roles`,
`jamfplatform_pro_api_role_privileges`.

List resources: `jamfplatform_pro_api_client`, `jamfplatform_pro_api_role`.

`jamfplatform_pro_api_role_privileges` has no replacement, as no endpoint serves the privilege
list. The privileges themselves are retained in a new form, described under
[API integration credentials](#api-integration-credentials).

### Actions (configuration edit only)

**Action needed: two renames, nothing else.** `POST /v2/mdm/commands` was unpublished at the
GA, which removed the fourteen MDM command actions that depended on it. Jamf Pro serves it
again, and the actions are back. Actions hold no state, so editing the `action` block is the
whole of the change.

- `jamfplatform_pro_device_lock` is now `jamfplatform_pro_lock_device`.
- On `jamfplatform_pro_enable_lost_mode`, `lost_mode_message`, `lost_mode_phone` and
  `lost_mode_footnote` are now `message`, `phone_number` and `footnote`.

Every other restored action keeps its name and attributes, except
`jamfplatform_pro_set_auto_admin_password`, which now requires both `guid` and `password`.
`jamfplatform_pro_clear_passcode` now fails with a named error when Jamf Pro holds no unlock
token for the device and you supplied none, because Jamf Pro refuses the command without one.

The restored set is `jamfplatform_pro_lock_device`, `jamfplatform_pro_enable_lost_mode`,
`jamfplatform_pro_disable_lost_mode`, `jamfplatform_pro_play_lost_mode_sound`,
`jamfplatform_pro_enable_remote_desktop`, `jamfplatform_pro_disable_remote_desktop`,
`jamfplatform_pro_clear_restrictions_password`, `jamfplatform_pro_clear_passcode`,
`jamfplatform_pro_delete_user`, `jamfplatform_pro_log_out_user`,
`jamfplatform_pro_unlock_user_account`, `jamfplatform_pro_set_auto_admin_password`,
`jamfplatform_pro_trigger_enhanced_log_collection` and
`jamfplatform_pro_cancel_enhanced_log_collection`.

New with the restoration: `jamfplatform_pro_set_wallpaper`, `jamfplatform_pro_set_recovery_lock`
and `jamfplatform_pro_clear_recovery_lock`. `jamfplatform_pro_send_blank_push`,
`jamfplatform_pro_renew_mdm_profile` and `jamfplatform_pro_flush_mdm_commands` were never affected.

## API integration credentials

**Action needed: register a replacement integration and update the provider credentials.** Beta
API integration credentials are revoked. A beta client cannot be migrated to a GA client, and both
the permission model and the endpoint paths have changed.

Credentials are registered in the Jamf Account Platform API integrations UI. A single OAuth 2.0
authentication model now covers the whole platform, in place of one credential set per product.
Step-by-step instructions are published here:

**[Getting started with the Platform API](https://developer.jamf.com/platform-api/reference/getting-started-with-platform-api)**

Refer to that page for how to register an integration and where to obtain its client ID and
secret.

**Permissions are organised by capability and action**, not by product privilege list. The API
names take the form `compliance-benchmarks:create` or `device-groups:read`, and Jamf Account's
permission picker presents the same thing as a named permission with a checkbox per action.
Resource, data source, list resource and action pages in this documentation carry a **Required Jamf
permissions** table written the way the picker reads: the section, the permission name, the boxes
to tick, and the Platform API capability behind them. Use those tables to select permissions for
the replacement integration, and grant only what the constructs in use require.

Grant with two consequences of the new model in mind. An action covers only itself, so an
integration that reads a record before modifying it needs the read action as well as the update
action. And the pre-GA computer and mobile privilege pairs have collapsed into single device-level
permissions: `devices`, `device-groups`, `extension-attributes`, `configuration-profiles`,
`enrollment-invitations`, `advanced-device-searches` and `prestage-enrollments`. A computers-only
integration is no longer expressible. The
**[Jamf Pro permissions map](https://developer.jamf.com/platform-api/reference/jamf-pro-permissions-map)**
is the reference for the full mapping, including the reverse case: the old computer and mobile
command privileges split into `device-actions` and `destructive-device-actions`, the latter
covering erase, unmanage and remove MDM profile.

**Three scope levels are available.**

A *platform environment* is a group of tenants across product types. Prefer it, for two concrete
reasons: one environment-scoped integration covers the whole group, and it is the only scope the
Platform services are published at. The Platform API GA deleted the tenant header from six of their
specifications outright, so environment scope is where that surface is going, one service at a
time.

A *tenant* scope targets a single Jamf Pro, Jamf School, Jamf Protect or Jamf Security Cloud
tenant. It is the legacy method of targeting an integration without a platform
environment. It costs one integration per tenant per product: a Jamf Pro tenant and a Jamf Protect
tenant are two integrations to create and two credential pairs to rotate. Treat it as the
exception, for a deliberately single-product integration, and note that it no longer reaches every
construct — see the list below.

An *organization management* scope reaches organization-level resources, the
`jamfplatform_account_*` family, and is the only scope that reaches them. Configure it by setting
*neither* `environment_id` nor `tenant_id`: the gateway resolves the organization from the access
token.

Every resource and data source reports the scope it needs when it is configured, so pointing an
integration at a family it cannot reach is named at plan time, not mid-apply. The scope you select
determines the provider configuration described under [Scope](#scope).

## Base URL

**Action needed: change the host.** The gateway host and the provider version are paired in both
directions:

| Provider version | `base_url` |
|---|---|
| `v0.28.1` and earlier | `https://{region}.apigw.jamf.com`, the beta host, which stopped serving the API. These versions cannot reach the GA host. |
| `v0.29.0-rc.4` through `rc.7` | `https://{region}.api.jamfcloud.com`, already in place. The host needs no change, but the credential and scope replacement still do. |
| `v0.29.0` | `https://{region}.api.jamfcloud.com`, required. This version cannot reach the beta host. |

Change the host and upgrade the provider in a single change: neither the GA host on an earlier
version nor the beta host on `v0.29.0` is a supported combination, and the beta host no longer
answers in any case.

```hcl
provider "jamfplatform" {
  base_url = "https://us.api.jamfcloud.com" # or eu., or apac.
}
```

The value may also be supplied through `JAMFPLATFORM_BASE_URL`. The region remains mandatory.

Supply the host only. The gateway serves the token endpoint and every API namespace at the root,
and the `/api` path segment used during the beta is gone. A request carrying it gets the gateway's
bare `404 page not found`, not a JSON error.

The beta host no longer works with this provider. `v0.29.0` and `v0.30.0` let you set it anyway,
and you got a plan offering to create everything you already manage. Running that plan changed
nothing in Jamf, since every create failed as well. It emptied your state file, so recover that
from `terraform.tfstate.backup` or your backend's state history. This release refuses the beta
host.

## Scope

**Action needed for most configurations: replace `tenant_id` with `environment_id`.** This change
goes with the credential replacement, because the attribute has to match how the replacement
integration was registered.

The scope an API integration targets now travels in a request header, not a URL path, and the
provider has gained an `environment_id` attribute alongside `tenant_id`. Public-beta integrations
were tenant-scoped, so most configurations being upgraded set `tenant_id` or export
`JAMFPLATFORM_TENANT_ID`. A replacement integration will usually be environment-scoped, so
registering it comes with a provider configuration change:

```hcl
provider "jamfplatform" {
  base_url = "https://eu.api.jamfcloud.com"
  # tenant_id      = var.jamf_tenant_id
  environment_id = var.jamf_environment_id
}
```

Both attributes may also be supplied through `JAMFPLATFORM_ENVIRONMENT_ID` and
`JAMFPLATFORM_TENANT_ID`. They are mutually exclusive and both optional. An integration targets one
or the other, `environment_id` is preferred, and `tenant_id` is the legacy method of targeting
integrations without a platform environment. A tenant-scoped GA integration remains valid for
single-product access: `jamfplatform_pro_*` and `jamfplatform_security_cloud_*` work under either
scope, and so do the Platform inventory constructs — `jamfplatform_device`,
`jamfplatform_devices`, `jamfplatform_device_group`, `jamfplatform_device_groups` and the
`jamfplatform_device_*` actions. Three sets of constructs are out of its reach, each for a
different reason:

- **AI Governance** is refused by the provider, at configure time, with a diagnostic naming the
  construct. `jamfplatform_ai_governance_policy` and the tool catalogue require environment scope.
- **Blueprints and compliance benchmarks** are refused by the provider, at configure time, with the
  same diagnostic. Their specifications deleted the tenant header at the GA, and their permissions
  cannot be selected when a tenant-scoped integration is created either, so such an integration
  could never hold them. This used to surface as `403 BAD_PERMISSIONS` mid-apply, which the
  provider could not attribute — a permission absent from the integration is indistinguishable from
  any other privilege gap — and is now named at configure time instead. Choose environment scope if
  the configuration manages either.
- **Jamf Account** requires a third scope, *organization management*, which neither of these
  attributes selects. `jamfplatform_account_*` is the only family that scope reaches, and it is the
  only scope that reaches the family. The provider refuses each direction at configure time with a
  diagnostic naming the construct. Configure it by setting **neither** `environment_id` nor
  `tenant_id` and exporting neither variable: the gateway resolves the organization from the access
  token, so no identifier is supplied. An organization-scoped integration therefore belongs in its
  own provider block, aliased, or its own workspace.

Each construct's documentation page names the scope its endpoints are published at, in the same
block as its permission table, taken from the API specifications the provider is generated against.
That is not always the same set as the provider enforces: where the gateway still serves a header a
specification has withdrawn, as it does for the Platform inventory constructs listed above, the
documentation page reports the specification and the provider goes on accepting the wider set. A
page naming environment scope is therefore not evidence that a working tenant-scoped integration
needs replacing — the configure-time diagnostic is what states the scopes a given provider release
accepts, and it is raised only when the configured scope is refused.

Three failure modes follow from the change, easiest to diagnose first:

- Retaining `tenant_id` alongside `environment_id` is rejected at configure time with a
  `Conflicting API Integration Scope` error. The old attribute must be removed, not left in place.
- A `JAMFPLATFORM_TENANT_ID` still exported in CI is ignored, with a warning naming it, when
  `environment_id` is set in the provider block. Where neither attribute is set in the provider
  block and both environment variables are exported, configuration fails with the same conflict
  error. Unset the tenant variable as part of the cutover.
- Supplying the identifier that does not correspond to how the integration was created is refused
  with `403 OWNERSHIP_FORBIDDEN`, even where both identifiers belong to the same customer.

Where a configuration must supply the Jamf Pro tenant identifier to another Jamf product, the
`jamfplatform_pro_tenant_id` data source resolves it from the configured scope, so the identifier
never has to be carried between consoles by hand. `jamfplatform_security_cloud_uem_connect` is the
case it was built for.

## Attribute removals and deprecations

### Removed: `target_device_group` on `jamfplatform_cbengine_benchmark`

**Action needed if the attribute is in use: switch to `target_device_groups`**, a set of device
group identifiers:

```hcl
resource "jamfplatform_cbengine_benchmark" "example" {
  # target_device_group  = jamfplatform_device_group.macs.id
  target_device_groups = [jamfplatform_device_group.macs.id]
}
```

State migrates automatically. The provider folds a singular value into the set during the state
upgrade, so the benchmark keeps targeting the same device group. The removal takes a configuration
edit only, never a re-scope. The attribute is gone from the corresponding data source too.

### Removed: `category_name` and `site_name` on `jamfplatform_pro_patch_software_title`

**Action needed if either attribute is referenced: read the name from the object that owns it.**
Both were read-only display names, and the Jamf Pro endpoints this resource now uses report
category and site by ID alone. Where the category is managed by Terraform, read the name from that
resource. Where it is not, which is the usual case since the assignment is commonly made in the
Jamf Pro UI, look it up by the ID the title reports:

```hcl
# the category is managed by Terraform
output "category_name" {
  # value = jamfplatform_pro_patch_software_title.example.category_name
  value = jamfplatform_pro_category.example.name
}

# the category is not managed by Terraform
data "jamfplatform_pro_category" "title" {
  id = jamfplatform_pro_patch_software_title.example.category_id
}

output "category_name" {
  value = data.jamfplatform_pro_category.title.name
}
```

`jamfplatform_pro_site` covers `site_name` the same way, selected by either `id` or `name`.

State migrates automatically. The provider strips both attributes during the state upgrade, so no
`terraform state` surgery is needed. They are gone from the corresponding data source too.

**Grant two further permissions.** The data source, the list resource and `terraform import`
resolve a title's `source_id` from the tenant's patch source catalogues, because the endpoints
this resource now uses name its patch source without numbering it. An integration reaching this
resource through any of those three paths needs **App lifecycle management → External patch
sources → Read** and **App lifecycle management → Internal patch sources → Read** alongside
**Patch titles**. Without them the read fails with `403 BAD_PERMISSIONS`, reported as `Unable to
determine the patch software title's source_id`. Planning and applying a title already in state is
unaffected: `source_id` is carried forward, never resolved again. The **Required Jamf
permissions** table on each of the three documentation pages lists the full set.

**`category_id` and `site_id` no longer accept `"0"`.** Use `-1`, the default, for "No category
assigned" and "NONE". A configuration setting either attribute to `"0"` is now rejected at plan
time. That is the reverse of the retired endpoint's convention, where `0` cleared the assignment
and `-1` was a silent no-op. A title whose state still holds `"0"` needs no state edit; the next
refresh reads it back as `-1`. Only a configuration setting the value explicitly has to change.

Otherwise the resource behaves as before. Patch software titles are now read, updated and deleted
through Jamf Pro's `/patch-software-title-configurations` endpoints, in place of the deprecated
ProClassic `/patchsoftwaretitles` ones, and every attribute that remains keeps its meaning.
`version_packages` still manages only the versions it names, but it now delivers that by reading
the title's current assignments and rewriting the whole set, instead of leaving the endpoint to
merge each version in turn. An assignment created in the Jamf Pro UI between that read and the
write can therefore be lost. Do not edit a title's **Definition** tab while an apply is in flight.

### Deprecated, not yet removed: the flat blueprint component attributes

**No action needed yet.** The flat top-level component attributes on
`jamfplatform_blueprints_blueprint`, together with the
top-level `activation_conditions` and `legacy_payloads`, are superseded by named, ordered
`component_blocks`. They remain present and functional, and may be removed on or after **22
October 2026**. Migrate ahead of that date. `terraform plan` reports every attribute that needs it.

### Now read-only: `unmanaged_sync_threshold` on `jamfplatform_security_cloud_uem_connect`

**Action needed if the attribute is set: remove it from your configuration.** Like
`personal_device_enrollment_type` below, this is a service behaviour change, not a provider
decision. Unlike that one it could not be retained, because it was breaking every apply.

Jamf Security Cloud does not apply an unmanaged threshold to a Jamf Pro connection. It takes device
status from Jamf Pro directly. Wire-probed 2026-09-01: every value sent for a `JAMF_PRO` connector
is accepted and then discarded, and the connector always reports `0`. The provider defaulted the
attribute to `3` and sent it on every write, so Terraform saw a value it had planned come back
different and failed the apply:

```
Error: Provider produced inconsistent result after apply
.unmanaged_sync_threshold: was cty.NumberIntVal(3), but now cty.NumberIntVal(0)
```

The attribute is now `Computed` and always reads `0`. Leaving it in a configuration is an error
(`Invalid Configuration for Read-Only Attribute`), so delete the line. No state edit is needed.
Nothing about how your devices are treated changes: the setting never took effect.

### Narrowed: the per-title attributes on `jamfplatform_pro_app_installer_titles`

**Action needed only if one of the dropped attributes is referenced.** The App Installer
constructs ship in `v0.29.0` — see
[Retained: the App Installer constructs](#retained-the-app-installer-constructs) below — but the
plural catalog data source no longer claims fields the catalog list endpoint does not return.

`titles[*]` previously carried the full per-title attribute set, the same one the singular data
source uses. The catalog list endpoint has only ever returned a seven-field summary, so the other
thirteen attributes were always empty: `architecture`, `availability_date`, `installer_package_hash`,
`installer_package_hash_type`, `language`, `launch_daemon_included`, `minimum_os_version`,
`notification_available`, `original_media_sources`, `package_signing_identity`, `short_version`,
`size_in_bytes` and `suppress_auto_update`.

`titles[*]` now carries `id`, `title_name`, `publisher`, `bundle_id`, `version`, `icon_url` and
`installation_path_shared`. Six of those were already there; `installation_path_shared` is an
addition Jamf Pro now reports, new to this data source in this release. Read the rest from the
singular data source, which calls the per-title endpoint that does return them:

```hcl
data "jamfplatform_pro_app_installer_titles" "catalog" {
  name_substring = "Jamf Composer"
}

# titles[0].minimum_os_version was always "" — read it here instead
data "jamfplatform_pro_app_installer_title" "composer" {
  id = data.jamfplatform_pro_app_installer_titles.catalog.titles[0].id
}
```

The singular data source is unchanged apart from three additions Jamf Pro now reports —
`installation_path_shared`, `media_source_type` and `original_terms_and_conditions` — and a new
optional `version` argument, which reads a historical version of a title rather than its current
one. Data sources hold no state, so there is nothing to edit beyond the references themselves.

### Retained: the App Installer constructs

**No action needed.** `jamfplatform_pro_app_installer`,
`jamfplatform_pro_app_installer_settings` and `jamfplatform_pro_app_installer_title`, with their
data sources and list resource, shipped in `v0.28.1` and ship in `v0.29.0`.

They were withdrawn from `v0.29.0-rc.4` through `rc.7`, because the endpoints behind them appeared
in no published specification. That changed during the GA cycle: App Installers is now a documented
Platform API surface of 23 operations, and the constructs are back.

Two schema descriptions were wrong and are corrected here: `quit_delay` is in minutes, not
seconds, and `selected_version` holds the version pinned while `update_behavior` is `MANUAL` and is
empty while it is `AUTOMATIC`, rather than always the latest available version. Jamf Pro behaves as
it always did, so re-check any configuration written against the old wording: `quit_delay = 300` is
five hours.

Both resources' state shapes are identical to `v0.28.1`, so a state file written by that version
plans clean with no edit and no state upgrade. Only a workspace that upgraded to one of the four
affected release candidates has anything to do: if the state entries were removed then, re-adopt
with [`terraform import`](https://developer.hashicorp.com/terraform/cli/commands/import) — the
deployment ID for `jamfplatform_pro_app_installer`, and `singleton` for
`jamfplatform_pro_app_installer_settings`.

### Retained: `personal_device_enrollment_type`

**No action needed.** On `jamfplatform_pro_user_initiated_enrollment_settings`. The deprecation
is Jamf Pro's, not the provider's: Jamf Pro has ignored the value since 11.25 and always reports
`USERENROLLMENT`. The attribute is read-only and is retained.

## Additions since v0.28.1

**No action needed.** The following arrived during the GA cycle. All of it is additive relative to
`v0.28.1`.

### Jamf Security Cloud

A new construct family, `jamfplatform_security_cloud_*`, reached through the same gateway and the
same credentials as the rest of the provider. A Security Cloud integration may be refused with
`403 NOT_ENTITLED` despite authenticating correctly. That is an entitlement gap, not a credential
problem, and the provider reports it as such.

| Area | Constructs |
|---|---|
| Custom DNS | `dns_zone` (with data sources and list resource), `dns_search_domain` and `dns_hostname_mappings` (each with a data source) |
| ZTNA gateways | `ztna_gateway` and `ztna_grouped_gateway` (both with data sources and list resources), `ztna_shared_gateways` data source |
| ZTNA access policy | `ztna_app` (with data sources and list resource), `ztna_predefined_apps` data source |
| Device groups | `security_cloud_device_group` (with data sources and list resource), distinct from the Platform Services `jamfplatform_device_group` |
| UEM Connect | `uem_connect` (with data source and list resource), plus the `uem_connect_synchronize` and `activation_profile_deploy` actions |
| Activation profiles | `activation_profile`, a deliberately small part of a profile, with no import support and no drift detection. Changing any setting replaces the profile and mints a new activation code |
| Catalogues | `content_categories` data source |

Two delete behaviours affect destroy ordering. A ZTNA gateway that is still referenced cannot be
deleted, and the provider names the referrer in the diagnostic instead of surfacing a bare `409`. A
Security Cloud device group referenced by a ZTNA app deletes successfully and empties the app's
assignment. The behaviour differs per construct.

Expect two further things on a first apply. A ZTNA gateway apply waits for the gateway to report
a settled status instead of returning the moment the write is accepted, so creating or
re-provisioning a dedicated internet gateway takes about five minutes. That wait is what makes its
`dedicated_egress_ip_addresses` usable from the same apply. Raise `create` or `update` in the
resource's `timeouts` block if a region provisions more slowly than the default ten minutes allows.

And **destroying a `uem_connect` connector in `platform_tenant` form leaves a live Jamf Pro API
integration behind.** Jamf Security Cloud creates one named `JSC Connector` to authenticate with,
nothing removes it, and the provider holds no Jamf Pro credentials for that tenant to remove it
with. The role carries 31 privileges, including fleet-wide profile and group writes. Audit
**Settings > API roles and clients** on the Jamf Pro instance after any destroy and delete the
orphans. Further detail: [Jamf Security Cloud](security-cloud).

### Jamf Account

The provider's first *organization-scoped* family, and the only one that scope reaches: configure
it by setting neither `environment_id` nor `tenant_id`, as described under [Scope](#scope). It is
served only from the US gateway.

`jamfplatform_account_sso_domain` claims a DNS domain for your Jamf Account organization's single
sign-on, with singular and plural data sources, a list resource, and the
`jamfplatform_account_sso_domain_verify` action that re-checks the domain's DNS ownership record.
`jamfplatform_account_sso_connection` manages the identity provider that
signs people in for those domains: Entra, Okta, Google Workspace, or any generic OpenID Connect
provider. It ships with singular and plural data sources and a list resource.

Two domain behaviours come from Jamf Account rather than from any design choice, and both will
surprise you. Jamf Account exposes no read, update or patch for a single domain, so every attribute is
`RequiresReplace` and `terraform import` takes the domain **name** in place of an ID. The ID would
not serve anyway: reclaiming a domain mints a new one.

Verification is an action, not resource state, and it never polls. A failed check returns `200`
with the status unchanged, so the status code tells you nothing. It still pushes the fourteen-day
deadline out. And the five-minute rate limit runs from the moment the domain was claimed, so the
first check straight after a create is always refused. Publish the `verification_txt_record` value
(exported whole, prefix included, as the console shows it), wait five minutes, then run the action.

Three connection behaviours are worth knowing before writing one. Jamf Account cannot currently
apply a change to an existing connection: every in-place write is refused, whatever it carries,
including the body a create accepts. So the provider plans a replacement for any change, rotating
`client_secret` included, and a connection carrying real sign-in traffic wants
`create_before_destroy` set on it. A connection name takes letters and digits only, and Jamf
Account appends a suffix the console hides, so two connections created under the same name are told
apart only by `internal_name` and `id`. And while the products a connection is enabled for are
returned, the tenants within them never are, which makes `enabled_products`
configuration-authoritative: a change made in the console is invisible to `terraform plan`.
Further detail:
[Jamf Account single sign-on](account-sso).

### Jamf AI Governance

`jamfplatform_ai_governance_policy` manages the settings delivered to an AI tool: Claude Code,
Claude Desktop and OpenAI Codex at present. A blueprint then delivers a pinned policy version to
Macs. It ships with singular and plural data sources and a list resource.
`jamfplatform_ai_governance_tool` and `jamfplatform_ai_governance_tools` read the product
catalogue. The settings body is the tool vendor's own JSON, validated at plan time against the
schema the platform serves. This is the one construct family the provider itself gates on
environment scope, described under [Scope](#scope). Further detail:
[AI Governance policies](ai-governance-policies).

### Elsewhere

- `environment_id` and the `jamfplatform_pro_tenant_id` data source, both described under
  [Scope](#scope).
- `custom_headers` and `authorization_header_name`, for networks in which Terraform reaches Jamf
  through a proxy that authenticates callers itself. `Cookie`, `Content-Type` and `Accept` are
  refused in both, because a supplied header replaces instead of adding and would displace a value
  the provider chose per request. See
  [Reverse proxies and custom headers](reverse-proxy).
- The `jamfplatform_pro_patch_policy` list resource now names any patch policy it could not
  enumerate, in a warning on the list result, instead of dropping it silently. A
  `plan -generate-config-out` run that came back one resource short of the tenant said nothing
  about it before.
- `jamfplatform_security_cloud_uem_connect` validates `sync_refresh_interval_minutes` at plan time
  against the intervals the service actually accepts (`60`, `120`, `240`, `480`, `720` and
  `1440`), instead of letting any other value fail the apply with an unattributed `422`.
- An incorrect `base_url` is now reported as such, rather than presenting as a network failure.
- Built against Jamf Pro 11.31.0 and Classic API 11.28.0.
- The required-permission tables throughout this documentation are regenerated against the GA
  capability model, and now name each permission as Jamf Account's permission picker does rather
  than by its retired Jamf Pro privilege name.
- Every schema description and example comment has been rewritten in a plainer, more consistent
  voice. No attribute, value, default or behaviour changed with it.

## Troubleshooting

| Symptom | Cause and resolution |
|---|---|
| `no schema available for <type>.<name> while reading state; this is a bug in Terraform and should be reported` | Not a Terraform defect. A removed resource remains in state; remove it with `terraform state rm`. Every operation in the workspace fails until it is removed. |
| `404 page not found`, with no JSON body | `base_url` includes a path, or names a host that does not serve the Platform API. Supply the regional gateway host with no path. |
| A plan offers to create objects that already exist, with no error | `base_url` still names the beta host, on `v0.29.0` or `v0.30.0`. Check the state with `terraform plan -refresh=false`, then change the host. This release refuses that host at configure time. |
| `403 OWNERSHIP_FORBIDDEN` | `environment_id` supplied for a tenant-scoped integration, or the reverse. |
| `403 BAD_PERMISSIONS` | The integration lacks a permission the construct requires; the resource's documentation page lists them, alongside the scope it needs. Blueprints and compliance benchmarks under a tenant-scoped integration used to land here; they are now refused at configure time instead. |
| `403 NOT_ENTITLED` on a Security Cloud construct | The tenant does not hold that Security Cloud capability. Additional permissions will not resolve it. |
| Authentication fails outright | Beta credentials in use. Register a replacement integration in Jamf Account. |
