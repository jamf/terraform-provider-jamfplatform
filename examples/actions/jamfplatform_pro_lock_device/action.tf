# Locks the listed computers or mobile devices. Take the PIN from a variable so
# it stays out of the configuration.
#
#   terraform apply -invoke='action.jamfplatform_pro_lock_device.lock'

variable "lock_pin" {
  type      = string
  sensitive = true
}

action "jamfplatform_pro_lock_device" "lock" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
    message        = "This Mac is locked. Please call IT."
    phone_number   = "+1 555 0100"
    pin            = var.lock_pin
  }
}
