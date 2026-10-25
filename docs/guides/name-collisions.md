---
page_title: "Name collisions"
description: |-
  Find a name that already belongs to an existing object during terraform plan, instead of failing halfway through apply.
---

# Name collisions

When a tenant is managed in the web interfaces and in Terraform, a configuration often creates an object whose name someone already used by hand. Jamf Pro refuses most of those writes, so `terraform plan` succeeds and `terraform apply` fails partway through. Some object types accept the duplicate, and the tenant ends up with two objects of one name.

The provider checks for the collision during `terraform plan`:

```
Error: Name already in use

  with jamfplatform_pro_category.apps,
  on main.tf line 3, in resource "jamfplatform_pro_category" "apps":
   3:   name = "Apps"

A category with the name "Apps" already exists in Jamf Pro (ID 12), and Jamf Pro
does not allow two with the same name. Names are compared without regard to case,
trailing spaces or accents.

To manage the existing category with Terraform, import it. Use the full address of this resource as `to`, including any module path:

  import {
    to = <this resource's address>
    id = "12"
  }

Otherwise, choose a different name. If this configuration already manages the
existing category under a different address, add a moved block instead.
```

The provider does not know where the resource sits in your configuration, so `to` is a placeholder. Terraform names the resource address in the diagnostic header. For a resource inside a module, use the full address, for example `module.apps.jamfplatform_pro_category.apps`.

The check runs again just before each resource is applied, so an object created in the web interface between plan and apply is still caught.

## Settings

```hcl
provider "jamfplatform" {
  name_collision_check = {
    mode               = "error" # "off", "warn" or "error"
    include_unenforced = false
  }
}
```

| Setting | Default | Environment variable |
|---|---|---|
| `mode` | `error` | `JAMFPLATFORM_NAME_COLLISION_CHECK_MODE` |
| `include_unenforced` | `false` | `JAMFPLATFORM_NAME_COLLISION_CHECK_INCLUDE_UNENFORCED` |

`warn` reports the collision and lets the plan proceed. `off` disables the check and its API reads.

`include_unenforced` extends the check to object types that accept duplicate names, such as blueprints. The apply would succeed, so this is a strictness choice.

If your tenant keeps duplicate names on purpose, leave `include_unenforced` off. Terraform can then manage those objects as before. To see the duplicates without blocking a plan, set `mode = "warn"` as well.

## How names are compared

Jamf Pro does not compare names the same way for every object type. Some fold case, trailing spaces and accents. Policies and computer groups fold case and trailing spaces but treat `Café` and `Cafe` as different names. User groups are case-sensitive. The provider applies each type's own rule, and the diagnostic states it.

Some object types share one pool of names. Computer groups, for example, share it with Platform device groups of type `COMPUTER`, so the check covers both.

## Checked object types

One row per construct. "Checked by default" lists the keys the service refuses a duplicate for, so the check is on unless you set `mode = "off"`. "Checked with `include_unenforced`" lists the keys the service accepts duplicates for. The provider compares those strictly, because the service stores every variant.

| Construct | Checked by default | Checked with `include_unenforced` | Compared without regard to |
|---|---|---|---|
| `jamfplatform_pro_category` | name | — | case, trailing spaces, accents |
| `jamfplatform_pro_script` | name | — | case, trailing spaces, accents |
| `jamfplatform_pro_package` | display name, file name | — | case, trailing spaces, accents |
| `jamfplatform_pro_computer_extension_attribute` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_mobile_device_extension_attribute` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_macos_configuration_profile` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_mobile_device_configuration_profile` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_policy` | name | — | case, leading and trailing spaces |
| `jamfplatform_pro_user_group` | name | — | leading and trailing spaces |
| `jamfplatform_pro_static_computer_group` | name | — | case |
| `jamfplatform_pro_smart_computer_group` | name | — | case |
| `jamfplatform_pro_static_mobile_device_group` | name | — | case, trailing spaces, accents |
| `jamfplatform_pro_smart_mobile_device_group` | name | — | case, trailing spaces, accents |
| `jamfplatform_device_group` | name, per device type | — | case, plus trailing spaces and accents for mobile groups |
| `jamfplatform_security_cloud_device_group` | name | — | leading and trailing spaces |
| `jamfplatform_security_cloud_dns_zone` | domain | name | domain: case; name: case, trailing spaces, accents |
| `jamfplatform_security_cloud_ztna_app` | hostname | name | hostname: case; name: case, trailing spaces, accents |
| `jamfplatform_account_sso_domain` | domain | — | case |
| `jamfplatform_cbengine_benchmark` | title | — | case |
| `jamfplatform_pro_app_installer` | — | name | case, trailing spaces, accents |
| `jamfplatform_pro_app_request_form_field` | — | title | case, trailing spaces, accents |
| `jamfplatform_pro_mobile_device_app` | — | name | case, leading and trailing spaces, accents |
| `jamfplatform_pro_mobile_device_enrollment_profile` | — | name | case, leading and trailing spaces, accents |
| `jamfplatform_pro_patch_external_source` | — | name | case, leading and trailing spaces, accents |
| `jamfplatform_pro_supervision_identity` | — | display name | case, trailing spaces, accents |
| `jamfplatform_pro_vpp_assignment` | — | name | case, leading and trailing spaces, accents |
| `jamfplatform_pro_vpp_invitation` | — | name | case, leading and trailing spaces, accents |
| `jamfplatform_blueprints_blueprint` | — | name | case, trailing spaces, accents |
| `jamfplatform_ai_governance_policy` | — | name | case, trailing spaces, accents |
| `jamfplatform_pro_enrollment_customization` | — | display name | case, trailing spaces, accents |
| `jamfplatform_pro_computer_prestage_enrollment` | — | display name | case, trailing spaces, accents |
| `jamfplatform_pro_mobile_device_prestage_enrollment` | — | display name | case, trailing spaces, accents |
| `jamfplatform_pro_advanced_computer_search` | — | name | case, leading and trailing spaces, accents |
| `jamfplatform_security_cloud_ztna_gateway` | — | name | case, trailing spaces, accents |
| `jamfplatform_security_cloud_ztna_grouped_gateway` | — | name | case, trailing spaces, accents |
| `jamfplatform_pro_building` | name | — | case, trailing spaces, accents |
| `jamfplatform_pro_department` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_site` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_dock_item` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_printer` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_webhook` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_network_segment` | name | — | case, leading and trailing spaces |
| `jamfplatform_pro_restricted_software` | name | — | case, leading and trailing spaces |
| `jamfplatform_pro_ebook` | name | — | case, leading and trailing spaces |
| `jamfplatform_account_platform_environment` | — | name | case, leading and trailing spaces, accents |
| `jamfplatform_pro_cloud_identity_provider` | display name | — | case, leading and trailing spaces, accents |

Jamf Security Cloud activation profiles and Jamf Account SSO connections are not checked. An activation profile read returns only its code, with no name to compare, and Jamf Account appends a random suffix to every connection name, so the stored name never equals the planned one.

Smart groups, static groups and Platform device groups of one device type share one pool of names, so a new computer group is checked against all three. Computer groups and mobile device groups are separate pools, so the same name can exist once in each. `jamfplatform_device_group` is checked against the pool for its own `device_type`.

## Values that are not known at plan time

Input variables, locals and data sources read at plan time are known when the check runs, so a name built from them is checked in `terraform plan`.

A name that depends on another resource's computed output, such as `"${random_id.suffix.hex}-apps"`, is unknown until that resource is applied. The plan skips it without a message. Terraform plans each resource again just before applying it, with the value resolved, and the check runs then. The error arrives at apply, before the write. It names the existing object and the `import` block. Resources applied earlier in the same run already exist.

Where a collection is partly unknown, such as a set of domains, the known entries are checked at plan time and the rest at apply.

## Cost and limits

- Each object type is listed once per plan and shared by every resource of that type. A tenant with thousands of objects of one type pays one paged read.
- A listing that fails produces one warning and the plan continues. The check never fails a plan it could not evaluate.
- Two resources in one configuration with the same name are not caught at plan time, because neither exists yet. The service refuses the second write during apply, as it does today.
- A `create_before_destroy` replacement that keeps the same name still fails at apply. The provider cannot see `lifecycle`.
- A name reused straight after a delete can be flagged until the old object is gone. This applies to eBooks, where the delete completes in the background.
