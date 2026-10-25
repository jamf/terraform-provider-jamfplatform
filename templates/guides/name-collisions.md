---
page_title: "Name collisions"
description: |-
  Find a name that already belongs to an existing object during terraform plan, instead of failing halfway through apply.
---

# Name collisions

When a tenant is managed in the web interfaces and in Terraform, a configuration often creates an object whose name someone already used by hand. Jamf Pro refuses most of those writes, so `terraform plan` succeeds and `terraform apply` fails partway through. For some object types Jamf accepts the duplicate and the tenant ends up with two objects of one name.

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

`include_unenforced` extends the check to object types where Jamf accepts duplicate names, such as blueprints. The apply would succeed, so this is a strictness choice and not a prediction of failure.

If your tenant keeps duplicate names on purpose, leave `include_unenforced` off. Terraform can then manage those objects as before. To see the duplicates without blocking a plan, set `mode = "warn"` as well.

## How names are compared

Jamf Pro does not compare names the same way for every object type. Some fold case, trailing spaces and accents. Policies and computer groups fold case and trailing spaces but treat `Café` and `Cafe` as different names. User groups are case-sensitive. The provider applies the rule Jamf applies for each type and the diagnostic states it.

Some object types share one pool of names. Computer groups, for example, share it with Platform device groups of type `COMPUTER`, so the check covers both.

## Checked object types

Each row is one uniqueness rule. "Refused by the service" says whether the service rejects a duplicate. A `Yes` row is checked by default, because the apply would fail anyway. A `No` row is checked only when `include_unenforced` is `true`, and the comparison there is the strict one the provider chose: the service stores every variant.

| Construct | Key | Refused by the service | Compared without regard to |
|---|---|---|---|
| `jamfplatform_pro_category` | name | Yes | case, trailing spaces, accents |
| `jamfplatform_pro_script` | name | Yes | case, trailing spaces, accents |
| `jamfplatform_pro_package` | display name | Yes | case, trailing spaces, accents |
| `jamfplatform_pro_package` | file name | Yes | case, trailing spaces, accents |
| `jamfplatform_pro_computer_extension_attribute` | name | Yes | case, leading and trailing spaces, accents |
| `jamfplatform_pro_mobile_device_extension_attribute` | name | Yes | case, leading and trailing spaces, accents |
| `jamfplatform_pro_macos_configuration_profile` | name | Yes | case, leading and trailing spaces, accents |
| `jamfplatform_pro_mobile_device_configuration_profile` | name | Yes | case, leading and trailing spaces, accents |
| `jamfplatform_pro_policy` | name | Yes | case, leading and trailing spaces |
| `jamfplatform_pro_user_group` | name | Yes | leading and trailing spaces |
| `jamfplatform_pro_static_computer_group` | name | Yes | case |
| `jamfplatform_pro_smart_computer_group` | name | Yes | case |
| `jamfplatform_pro_static_mobile_device_group` | name | Yes | case, trailing spaces, accents |
| `jamfplatform_pro_smart_mobile_device_group` | name | Yes | case, trailing spaces, accents |
| `jamfplatform_device_group` | name | Yes | case, plus trailing spaces and accents for mobile groups |
| `jamfplatform_security_cloud_device_group` | name | Yes | leading and trailing spaces |
| `jamfplatform_security_cloud_dns_zone` | domain | Yes | case |
| `jamfplatform_security_cloud_ztna_app` | hostname | Yes | case |
| `jamfplatform_account_sso_domain` | domain | Yes | case |
| `jamfplatform_cbengine_benchmark` | title | Yes | case |
| `jamfplatform_pro_app_installer` | name | No | case, trailing spaces, accents |
| `jamfplatform_pro_app_request_form_field` | title | No | case, trailing spaces, accents |
| `jamfplatform_pro_mobile_device_app` | name | No | case, leading and trailing spaces, accents |
| `jamfplatform_pro_mobile_device_enrollment_profile` | name | No | case, leading and trailing spaces, accents |
| `jamfplatform_pro_patch_external_source` | name | No | case, leading and trailing spaces, accents |
| `jamfplatform_pro_supervision_identity` | display name | No | case, trailing spaces, accents |
| `jamfplatform_pro_vpp_assignment` | name | No | case, leading and trailing spaces, accents |
| `jamfplatform_pro_vpp_invitation` | name | No | case, leading and trailing spaces, accents |

Smart groups, static groups and Platform device groups of one device type share one pool of names, so a new computer group is checked against all three.

## Values that are not known at plan time

Input variables, locals and data sources read at plan time are known when the check runs, so a name built from them is checked in `terraform plan`.

A name that depends on another resource's computed output, such as `"${random_id.suffix.hex}-apps"`, is unknown until that resource is applied. The plan skips it without a message. Terraform plans each resource again just before applying it, with the value resolved, and the check runs then. The failure arrives at apply, before the write, naming the existing object and the `import` block, but resources applied earlier in the same run are already created.

Where a collection is partly unknown, such as a set of domains, the known entries are checked at plan time and the rest at apply.

## Cost and limits

- Each object type is listed once per plan and shared by every resource of that type. A tenant with thousands of objects of one type pays one paged read.
- A listing that fails produces one warning and the plan continues. The check never fails a plan it could not evaluate.
- Two resources in one configuration with the same name are not caught at plan time, because neither exists yet. Jamf refuses the second write during apply, as before.
- A `create_before_destroy` replacement that keeps the same name still fails at apply. The provider cannot see `lifecycle`.
- A name reused straight after a delete can be flagged until Jamf finishes deleting the old object. This applies to eBooks, where the delete completes in the background.
