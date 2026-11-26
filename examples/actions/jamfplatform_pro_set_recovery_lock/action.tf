# Sets the Recovery Lock password on the listed Apple silicon Macs. Take the
# password from a variable.
#
#   terraform apply -invoke='action.jamfplatform_pro_set_recovery_lock.lock'

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
