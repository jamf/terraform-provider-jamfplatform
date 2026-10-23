variable "recovery_lock_password" {
  type      = string
  sensitive = true
}

action "jamfplatform_pro_set_recovery_lock" "lock" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
    password       = var.recovery_lock_password
  }
}
