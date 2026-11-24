# Erases one device. The erase cannot be undone, so the action never runs on a
# plain apply. Name it on the command line:
#
#   terraform apply -invoke='action.jamfplatform_device_erase.erase'
#
# Check the serial number in the data source before you invoke it.
data "jamfplatform_devices" "target" {
  filter = [
    {
      selector = "serialNumber"
      argument = "F2LXXXXXXXXX"
    }
  ]
}

variable "find_my_pin" {
  type      = string
  sensitive = true
  default   = null
}

action "jamfplatform_device_erase" "erase" {
  config {
    device_id = one(data.jamfplatform_devices.target.devices).id

    # Mobile devices only. Add return_to_service = true only when the tenant has
    # Return to Service configured, or Jamf refuses the erase.
    preserve_data_plan       = true
    disallow_proximity_setup = false
    clear_activation_lock    = true

    # Computers only: the six-character Find My PIN.
    pin = var.find_my_pin
  }
}
