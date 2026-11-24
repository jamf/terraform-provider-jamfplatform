# Stops an AppleCare enhanced log collection session on the listed devices.
#
#   terraform apply -invoke='action.jamfplatform_pro_cancel_enhanced_log_collection.stop'

action "jamfplatform_pro_cancel_enhanced_log_collection" "stop" {
  config {
    serial_numbers = ["F2LXXXXXXXXX"]
  }
}
