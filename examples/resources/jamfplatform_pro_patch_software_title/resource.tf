# A patch software title is defined by its catalog key (name_id) and patch
# source (source_id). Rather than hard-coding those, discover them from a patch
# source's available_titles catalog and build the title dynamically.

# Read the built-in "Jamf" source and its published catalog.
data "jamfplatform_pro_patch_internal_source" "jamf" {
  name = "Jamf"
}

locals {
  # The app we want to manage, matched against the catalog by display name.
  target_app = "8x8 Work"

  # Resolve the catalog entry (one() errors if it is missing or ambiguous).
  catalog_entry = one([
    for t in data.jamfplatform_pro_patch_internal_source.jamf.available_titles : t
    if t.app_name == local.target_app
  ])
}

resource "jamfplatform_pro_patch_software_title" "eight_by_eight" {
  # name_id and source_id are derived from the catalog lookup above. No magic
  # numbers. source_id is numeric; the data source id is a string, so convert it.
  name      = local.catalog_entry.app_name
  name_id   = local.catalog_entry.name_id
  source_id = tonumber(data.jamfplatform_pro_patch_internal_source.jamf.id)

  category_id        = "-1"
  site_id            = "-1"
  web_notification   = true
  email_notification = false

  # Jamf Pro populates available_versions; assign packages to specific versions
  # via version_packages so patch policies can target them. Only the versions
  # you list are managed; removing a key clears that version's package.
  version_packages = {
    "8.33.2.2" = jamfplatform_pro_package.work_8_33.id
  }
}

resource "jamfplatform_pro_package" "work_8_33" {
  display_name = "8x8 Work 8.33.2.2"
  file_name    = "8x8-work-8.33.2.2.pkg"
}

# Some titles (e.g. Adobe AIR) ship a Jamf extension attribute that collects the
# installed version on managed computers; inventory is not gathered until it is
# accepted. Set accept_extension_attributes = true to accept any pending ones.
# Accepting is one-way and cannot be reverted. The read-only extension_attributes
# list reports each EA and its acceptance status.
resource "jamfplatform_pro_patch_software_title" "adobe_air" {
  name      = "Adobe AIR"
  name_id   = "0AE"
  source_id = tonumber(data.jamfplatform_pro_patch_internal_source.jamf.id)

  accept_extension_attributes = true
}

output "adobe_air_extension_attributes" {
  value = jamfplatform_pro_patch_software_title.adobe_air.extension_attributes
}

# Manage several titles from the same catalog, then build a smart group per
# title. A smart group criterion named "Patch Reporting: <title>" matches the
# catalog name, which can differ from name, so it reads software_title_name.
# The reference also makes Terraform create each title before its group.
locals {
  patch_apps = ["Audacity 4", "Mozilla Firefox"]

  catalog = {
    for app in local.patch_apps : app => one([
      for t in data.jamfplatform_pro_patch_internal_source.jamf.available_titles : t
      if t.app_name == app
    ])
  }
}

resource "jamfplatform_pro_patch_software_title" "apps" {
  for_each = local.catalog

  name      = each.value.app_name
  name_id   = each.value.name_id
  source_id = tonumber(data.jamfplatform_pro_patch_internal_source.jamf.id)
}

resource "jamfplatform_pro_smart_computer_group" "outdated" {
  for_each = jamfplatform_pro_patch_software_title.apps

  name = "${each.value.name} out of date"
  criteria = [
    {
      name        = "Patch Reporting: ${each.value.software_title_name}"
      search_type = "less than"
      value       = "Latest Version"
    },
  ]
}
