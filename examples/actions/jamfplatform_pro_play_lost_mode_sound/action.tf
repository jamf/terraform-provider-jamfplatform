# Plays the Lost Mode sound on the listed supervised iPhones and iPads so someone
# can find them. They must already be in Lost Mode.
#
#   terraform apply -invoke='action.jamfplatform_pro_play_lost_mode_sound.ping'

action "jamfplatform_pro_play_lost_mode_sound" "ping" {
  config {
    serial_numbers = ["F2LXXXXXXXXX"]
  }
}
