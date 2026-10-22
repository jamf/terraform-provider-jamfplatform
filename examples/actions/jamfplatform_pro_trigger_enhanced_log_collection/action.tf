variable "applecare_token" {
  type      = string
  sensitive = true
}

action "jamfplatform_pro_trigger_enhanced_log_collection" "collect" {
  config {
    # Target only the device AppleCare issued the token for.
    serial_numbers   = ["F2LXXXXXXXXX"]
    apple_care_token = var.applecare_token
  }
}
