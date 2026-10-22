action "jamfplatform_pro_enable_lost_mode" "lost" {
  config {
    serial_numbers = ["F2LXXXXXXXXX"]

    # Set message, phone_number, or both. Every targeted device shows the same text.
    message      = "This iPad is lost. Please call the number below."
    phone_number = "+1 555 0100"
    footnote     = "Reward offered"
  }
}
