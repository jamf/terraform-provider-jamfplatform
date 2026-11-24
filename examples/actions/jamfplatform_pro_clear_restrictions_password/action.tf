# Clears the Screen Time (restrictions) passcode on the listed iPhones and iPads.
#
#   terraform apply -invoke='action.jamfplatform_pro_clear_restrictions_password.reset'

action "jamfplatform_pro_clear_restrictions_password" "reset" {
  config {
    serial_numbers = ["F2LXXXXXXXXX"]
  }
}
