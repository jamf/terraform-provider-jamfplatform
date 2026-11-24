# Turns Lost Mode off on the listed supervised iPhones and iPads.
#
#   terraform apply -invoke='action.jamfplatform_pro_disable_lost_mode.found'

action "jamfplatform_pro_disable_lost_mode" "found" {
  config {
    serial_numbers = ["F2LXXXXXXXXX"]
  }
}
