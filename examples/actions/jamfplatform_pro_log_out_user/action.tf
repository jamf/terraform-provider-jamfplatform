# Logs the current user out of the listed Shared iPads.
#
#   terraform apply -invoke='action.jamfplatform_pro_log_out_user.sign_out'

action "jamfplatform_pro_log_out_user" "sign_out" {
  config {
    serial_numbers = ["F2LXXXXXXXXX"]
  }
}
