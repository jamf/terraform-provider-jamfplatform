# Break-glass: force-stop a stuck Managed Software Updates enable/disable process.
# Use only when jamfplatform_pro_managed_software_update reports that the feature did not
# finish turning on or off. Takes no input. Invoke it by name:
#
#   terraform apply -invoke='action.jamfplatform_pro_managed_software_update_abandon.unstick'

action "jamfplatform_pro_managed_software_update_abandon" "unstick" {
  config {}
}
