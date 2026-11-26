# Turns Remote Desktop on for the listed computers.
#
#   terraform apply -invoke='action.jamfplatform_pro_enable_remote_desktop.remote'

action "jamfplatform_pro_enable_remote_desktop" "remote" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
  }
}
