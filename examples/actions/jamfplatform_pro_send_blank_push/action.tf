# Nudge every member of a static computer group each time its membership
# changes. caller is the group that fired the trigger, and caller.members is its
# list of management IDs.
resource "jamfplatform_device_group" "pilot" {
  name        = "Pilot Macs"
  group_type  = "static"
  device_type = "computer"
  members     = [for d in data.jamfplatform_devices.macs.devices : d.id]

  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.jamfplatform_pro_send_blank_push.nudge_group]
    }
  }
}

data "jamfplatform_devices" "macs" {
  filter = [
    {
      selector = "modelIdentifier"
      argument = "Mac*"
    }
  ]
}

action "jamfplatform_pro_send_blank_push" "nudge_group" {
  config {
    management_ids = caller.members
  }
}

# Nudge named devices on demand:
#
#   terraform apply -invoke='action.jamfplatform_pro_send_blank_push.nudge_one'
action "jamfplatform_pro_send_blank_push" "nudge_one" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
  }
}
