action "jamfplatform_pro_set_wallpaper" "brand" {
  config {
    serial_numbers = ["F2LXXXXXXXXX"]
    location       = "lock_and_home_screens"

    # Use a picture from disk, or an icon already in Jamf Pro with icon_id.
    image = filebase64("${path.module}/wallpaper.png")
  }
}
