# Unlocks a local user account on the listed Macs.
#
#   terraform apply -invoke='action.jamfplatform_pro_unlock_user_account.unlock'

action "jamfplatform_pro_unlock_user_account" "unlock" {
  config {
    serial_numbers = ["C02XXXXXXXXX"]
    user_name      = "alex"
  }
}
