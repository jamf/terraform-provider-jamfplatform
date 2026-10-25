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

The category with the name "Apps" already exists in Jamf Pro (ID 12), and Jamf Pro
does not allow two with the same name. Names are compared without regard to case,
trailing spaces or accents.

To manage the existing category with Terraform, import it. Use the full address
of this resource as `to`, including any module path:

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

Jamf Pro does not compare names the same way for every object type, and one pool of names can compare differently depending on the API that writes to it. Most types fold case and trailing spaces. Many also fold accents, and Classic types fold a leading space too. Policies and Classic groups treat `Café` and `Cafe` as different names. Computer groups written through the JSON APIs fold case only. User groups are case-sensitive. The provider applies each type's own rule, and the diagnostic states it.

Smart groups, static groups and Platform device groups of one device type share one pool of names, so a new computer group is checked against all three. Computer groups and mobile device groups are separate pools, so the same name can exist once in each. `jamfplatform_device_group` is checked against the pool for its own `device_type`.

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
| `jamfplatform_pro_class` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_user_extension_attribute` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_ibeacon` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_disk_encryption_configuration` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_directory_binding` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_ldap_server` | display name | — | case, leading and trailing spaces |
| `jamfplatform_pro_account_group` | display name | — | case, leading and trailing spaces |
| `jamfplatform_pro_account` | username, email address | — | case, trailing spaces, accents |
| `jamfplatform_pro_advanced_user_search` | name | — | case, leading and trailing spaces |
| `jamfplatform_pro_removable_mac_address` | value | — | case, leading and trailing spaces |
| `jamfplatform_pro_licensed_software` | name | — | case, leading and trailing spaces |
| `jamfplatform_pro_allowed_file_extension` | extension | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_mac_app_store_app` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_advanced_mobile_device_search` | name | — | case |
| `jamfplatform_pro_advanced_volume_purchasing_content_search` | name | — | nothing, names are compared exactly |
| `jamfplatform_pro_file_share_distribution_point` | name | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_inventory_preload_record` | serial number | — | case, trailing spaces, accents |
| `jamfplatform_pro_patch_software_title` | name ID, patch source and site | — | nothing, compared exactly |
| `jamfplatform_pro_patch_policy` | name, within one software title | — | case, leading and trailing spaces, accents |
| `jamfplatform_pro_mobile_device_provisioning_profile` | name | — | case, leading and trailing spaces |

## Object types that are not checked

Every other construct in the provider is listed here with the reason. A test fails when a construct is in neither table, so a new one has to be wired or listed.

| Constructs | Why |
|---|---|
| `jamfplatform_pro_access_management_settings`, `jamfplatform_pro_activation_code`, `jamfplatform_pro_app_installer_settings`, `jamfplatform_pro_app_request_settings`, `jamfplatform_pro_computer_check_in_settings`, `jamfplatform_pro_computer_inventory_collection_settings`, `jamfplatform_pro_gsx_connection_settings`, `jamfplatform_pro_impact_alert_notification_settings`, `jamfplatform_pro_jamf_connect`, `jamfplatform_pro_jamf_parent_settings`, `jamfplatform_pro_jamf_protect`, `jamfplatform_pro_jamf_teacher_settings`, `jamfplatform_pro_local_admin_password_settings`, `jamfplatform_pro_login_page_settings`, `jamfplatform_pro_macos_onboarding`, `jamfplatform_pro_managed_software_update`, `jamfplatform_pro_mdm_profile_settings`, `jamfplatform_pro_re_enrollment_settings`, `jamfplatform_pro_return_to_service`, `jamfplatform_pro_self_service_branding_ios`, `jamfplatform_pro_self_service_branding_macos`, `jamfplatform_pro_self_service_macos_settings`, `jamfplatform_pro_self_service_plus_settings`, `jamfplatform_pro_service_discovery_enrollment`, `jamfplatform_pro_smtp_server`, `jamfplatform_pro_sso_failover_url`, `jamfplatform_pro_sso_settings`, `jamfplatform_pro_user_initiated_enrollment_settings`, `jamfplatform_pro_volume_purchasing_notification` | Settings with one record per tenant, so no second object can take a name. |
| `jamfplatform_pro_cloud_distribution_point`, `jamfplatform_pro_pki_json_web_token_configuration`, `jamfplatform_security_cloud_uem_connect` | One per tenant. A create against an existing one is refused, and the resources already point you at an import. |
| `jamfplatform_security_cloud_dns_search_domain`, `jamfplatform_security_cloud_dns_hostname_mappings` | The whole tenant setting is replaced with `PUT`, so there is no create. |
| `jamfplatform_pro_computer_invitation`, `jamfplatform_pro_mobile_device_invitation` | The service mints the invitation code and ignores the one you supply. |
| `jamfplatform_pro_automated_device_enrollment`, `jamfplatform_pro_volume_purchasing_location` | A duplicate collides on the server token or the Apple location behind it, not on the name. |
| `jamfplatform_security_cloud_activation_profile` | A read returns only the profile code, with no name to compare. |
| `jamfplatform_account_sso_connection` | Jamf Account appends a random suffix to every connection name, so the stored name never equals the planned one. |
| `jamfplatform_pro_pki_adcs`, `jamfplatform_pro_pki_digicert`, `jamfplatform_pro_pki_venafi` | The provider has no way to list existing integrations, so there is nothing to compare against. |
| `jamfplatform_pro_icon`, `jamfplatform_pro_self_service_branding_image` | Uploads with no name key. |

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
