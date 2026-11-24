# Shuts one device down. A powered-off device receives no commands
# until someone starts it again.
#
#   terraform apply -invoke='action.jamfplatform_device_shutdown.power_off'

data "jamfplatform_devices" "target" {
  filter = [
    {
      selector = "serialNumber"
      argument = "C02XXXXXXXXX"
    }
  ]
}

action "jamfplatform_device_shutdown" "power_off" {
  config {
    device_id = one(data.jamfplatform_devices.target.devices).id
  }
}
