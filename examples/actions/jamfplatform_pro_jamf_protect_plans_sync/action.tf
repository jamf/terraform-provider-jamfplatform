# Trigger an on-demand sync of the Jamf Protect plans catalog into Jamf Pro
# (Settings → Jamf apps → Jamf Protect → Sync Plans). Requires the tenant to be
# registered with a Jamf Protect instance (jamfplatform_pro_jamf_protect).
# Takes no input. Invoke it by name:
#
#   terraform apply -invoke='action.jamfplatform_pro_jamf_protect_plans_sync.sync'

action "jamfplatform_pro_jamf_protect_plans_sync" "sync" {
  config {}
}
