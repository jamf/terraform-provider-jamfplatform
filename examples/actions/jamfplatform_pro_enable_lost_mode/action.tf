# Turns Lost Mode on for the listed supervised iPhones and iPads.
#
#   terraform apply -invoke='action.jamfplatform_pro_enable_lost_mode.lost'

action "jamfplatform_pro_enable_lost_mode" "lost" {
  config {
    serial_numbers = ["F2LXXXXXXXXX"]

    # Set message, phone_number, or both.
    message      = "This iPad is lost. Please call the number below."
    phone_number = "+1 555 0100"
    footnote     = "Reward offered"
  }
}
