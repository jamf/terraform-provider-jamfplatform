# Copyright Jamf Software LLC 2026
# SPDX-License-Identifier: MPL-2.0

# Minimal Computer PreStage Enrollment. `device_enrollment_program_instance_id`
# must be the ID of an ADE instance already provisioned in Jamf Pro.
resource "jamfplatform_pro_computer_prestage_enrollment" "example" {
  display_name                          = "Standard macOS PreStage"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = "1"
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  skip_setup_items = {
    filevault                = true
    all_your_files_in_icloud = true
    siri                     = true
    liquid_glass             = true
  }

  location_information   = {}
  purchasing_information = {}
  account_settings = {
    payload_configured                           = true
    local_admin_account_enabled                  = true
    admin_username                               = "ladmin"
    admin_password                               = "ChangeMeNow!"
    admin_password_wo_version                    = 1
    user_account_type                            = "ADMINISTRATOR"
    prefill_primary_account_info_feature_enabled = true
    prefill_type                                 = "DEVICE_OWNER"
  }

  scope_serial_numbers = []
}
