# Restarts one device. An action runs only when you invoke it or a resource
# trigger fires it, so a plain apply restarts nothing.
#
#   terraform apply -invoke='action.jamfplatform_device_restart.restart'

data "jamfplatform_devices" "target" {
  filter = [
    {
      selector = "serialNumber"
      argument = "C02XXXXXXXXX"
    }
  ]
}

action "jamfplatform_device_restart" "restart" {
  config {
    device_id = one(data.jamfplatform_devices.target.devices).id
  }
}
