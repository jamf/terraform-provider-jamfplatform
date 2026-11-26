# Turns Remote Desktop off on the listed computers.
#
#   terraform apply -invoke='action.jamfplatform_pro_disable_remote_desktop.remote'

action "jamfplatform_pro_disable_remote_desktop" "remote" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
  }
}
