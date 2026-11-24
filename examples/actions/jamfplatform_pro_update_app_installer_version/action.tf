# Run on demand, once the title publishes a newer version:
#
#   terraform apply -invoke='action.jamfplatform_pro_update_app_installer_version.move'

# Read the versions Jamf Pro publishes for the title.
data "jamfplatform_pro_app_installer_title" "composer" {
  id = "Composer"
}

# Move a MANUAL deployment on to the newest published version. The operation is
# forward-only: Jamf Pro refuses any version that is not newer than the
# deployment's current selected_version, including that version itself.
action "jamfplatform_pro_update_app_installer_version" "move" {
  config {
    deployment_id = jamfplatform_pro_app_installer.composer.id
    version       = one(reverse(data.jamfplatform_pro_app_installer_title.composer.versions)).version
  }
}
