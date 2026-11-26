# Enforce the latest macOS on a group every time the group is created or
# changes. caller.jamf_pro_id is the group's numeric Jamf Pro ID. A plan is a
# fire-once directive, so keep the trigger on events you want to repeat it for.
resource "jamfplatform_device_group" "pilot" {
  name        = "Pilot Macs"
  group_type  = "static"
  device_type = "computer"

  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.jamfplatform_pro_managed_software_update_plan.enforce_latest]
    }
  }
}

action "jamfplatform_pro_managed_software_update_plan" "enforce_latest" {
  config {
    group_id    = caller.jamf_pro_id
    object_type = "COMPUTER_GROUP" # or MOBILE_DEVICE_GROUP

    update_action = "DOWNLOAD_INSTALL"
    version_type  = "LATEST_ANY"

    # Required only when version_type is SPECIFIC_VERSION or CUSTOM_VERSION:
    # specific_version = "15.1"

    # Only valid when version_type is CUSTOM_VERSION. Jamf Pro rejects a build
    # version for every other version_type, including SPECIFIC_VERSION:
    # build_version = "21F79"

    # Optional, by update_action:
    # force_install_local_date_time = "2026-07-01T09:00:00" # DOWNLOAD_INSTALL_SCHEDULE
    # max_deferrals                 = 3                     # DOWNLOAD_INSTALL_ALLOW_DEFERRAL, 0–99
  }
}
