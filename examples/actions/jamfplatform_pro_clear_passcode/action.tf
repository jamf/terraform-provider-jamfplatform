# Clears the passcode on one iPhone or iPad.
#
#   terraform apply -invoke='action.jamfplatform_pro_clear_passcode.reset'

action "jamfplatform_pro_clear_passcode" "reset" {
  config {
    serial_number = "F2LXXXXXXXXX"

    # Leave unlock_token out and Jamf Pro looks it up from the device's inventory.
  }
}
