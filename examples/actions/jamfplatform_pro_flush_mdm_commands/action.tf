# Clear pending and failed MDM commands for a computer group whenever the group
# changes. The action takes the group's numeric Jamf Pro ID, which
# jamf_pro_id exposes, so caller.jamf_pro_id is all it needs.
resource "jamfplatform_device_group" "pilot" {
  name        = "Pilot Macs"
  group_type  = "static"
  device_type = "computer"

  lifecycle {
    action_trigger {
      events  = [after_update]
      actions = [action.jamfplatform_pro_flush_mdm_commands.clear_stuck]
    }
  }
}

action "jamfplatform_pro_flush_mdm_commands" "clear_stuck" {
  config {
    id_type = "computergroups"
    id      = caller.jamf_pro_id
    status  = "Pending+Failed"
  }
}

# Clear failed commands for one computer on demand. This id is the numeric Jamf
# Pro computer ID.
#
#   terraform apply -invoke='action.jamfplatform_pro_flush_mdm_commands.one_computer'
action "jamfplatform_pro_flush_mdm_commands" "one_computer" {
  config {
    id_type = "computers"
    id      = "123"
    status  = "Failed"
  }
}
