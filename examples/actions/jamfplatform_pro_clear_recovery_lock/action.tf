# Removes the Recovery Lock password from the listed Apple silicon Macs.
#
#   terraform apply -invoke='action.jamfplatform_pro_clear_recovery_lock.unlock'

action "jamfplatform_pro_clear_recovery_lock" "unlock" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
  }
}
