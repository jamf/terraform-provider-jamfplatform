# Renews the MDM enrollment profile on the listed mobile devices.
#
#   terraform apply -invoke='action.jamfplatform_pro_renew_mdm_profile.renew'

action "jamfplatform_pro_renew_mdm_profile" "renew" {
  config {
    # Hardware UDIDs, e.g. data.jamfplatform_device.example.hardware_udid
    udids = [
      "00000000000000000000000000000000000000000",
    ]
  }
}
