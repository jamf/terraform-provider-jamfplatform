# Copyright Jamf Software LLC 2026
# SPDX-License-Identifier: MPL-2.0

resource "jamfplatform_pro_mobile_device_prestage_enrollment" "example" {
  display_name                          = "Standard iPad PreStage"
  device_enrollment_program_instance_id = "1"

  mandatory              = true
  mdm_removable          = false
  require_authentication = false
  supervised             = true
  allow_pairing          = true
  auto_advance_setup     = false
  default_prestage       = false

  support_phone_number  = "+1-555-0100"
  support_email_address = "helpdesk@example.com"
  department            = "Field Operations"

  # "-1" = no site, "0" = no enrollment customization.
  enrollment_site_id          = "-1"
  enrollment_customization_id = "0"

  language = "en"
  region   = "US"
  timezone = "America/Chicago"

  prevent_activation_lock             = true
  enable_device_based_activation_lock = false

  prestage_minimum_os_target_version_type_ios  = "NO_ENFORCEMENT"
  prestage_minimum_os_target_version_type_ipad = "NO_ENFORCEMENT"

  # Shared iPad. storage_quota_size_megabytes is read-only (Computed): Jamf Pro
  # recalculates it server-side, so set it in the Jamf Pro admin UI, not here.
  # use_storage_quota_size and temporary_session_only are mutually exclusive.
  multi_user              = true
  maximum_shared_accounts = 8
  use_storage_quota_size  = true
  temporary_session_only  = false

  # Device-naming block ("List of Names" mode): Jamf Pro assigns each entry an
  # id and consumes them in order as devices enrol.
  names = {
    assign_names_using = "List of Names"
    manage_names       = true

    prestage_device_names = [
      { device_name = "ipad-lobby-01" },
      { device_name = "ipad-lobby-02" },
    ]
  }

  skip_setup_items = {
    location             = true
    apple_id             = true
    screen_time          = true
    siri                 = true
    device_features_tour = true
    liquid_glass         = true
  }

  location_information = {
    username = "fieldops"
    realname = "Field Operations"
  }

  purchasing_information = {
    purchased = true
    vendor    = "Apple"
  }

  # Folded scope: serial numbers must already exist on the ADE token.
  scope_serial_numbers = [
    "MNQ2KD6422",
  ]
}
