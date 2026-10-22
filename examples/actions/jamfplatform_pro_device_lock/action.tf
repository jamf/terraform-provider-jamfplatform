variable "lock_pin" {
  type      = string
  sensitive = true
}

action "jamfplatform_pro_device_lock" "lock" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
    message        = "This Mac is locked. Please call IT."
    phone_number   = "+1 555 0100"
    pin            = var.lock_pin
  }
}
