# Removes the device from management. Check the target before you invoke it.
#
#   terraform apply -invoke='action.jamfplatform_device_unmanage.retire'

data "jamfplatform_devices" "target" {
  filter = [
    {
      selector = "serialNumber"
      argument = "C02XXXXXXXXX"
    }
  ]
}

action "jamfplatform_device_unmanage" "retire" {
  config {
    device_id = one(data.jamfplatform_devices.target.devices).id
  }
}
