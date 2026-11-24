# Invoke by name, for example:
#
#   terraform apply -invoke='action.jamfplatform_security_cloud_uem_connect_synchronize.now'

# Starts a UEM Connect sync immediately instead of waiting for the next scheduled
# one. Jamf Security Cloud runs it in the background, so this returns as soon as the
# run has started and reports nothing about what it did.

# A tenant holds at most one UEM Connect integration, so uem_connect_id can be
# omitted entirely.
action "jamfplatform_security_cloud_uem_connect_synchronize" "now" {
  config {}
}

# Naming the resource's id makes the synchronize wait for the integration to
# exist. Useful for pulling inventory in straight after setup, rather than up to
# a full refresh interval later.
action "jamfplatform_security_cloud_uem_connect_synchronize" "after_setup" {
  config {
    uem_connect_id = jamfplatform_security_cloud_uem_connect.jamf_pro.id
  }
}

# Read the outcome from the data source; the action does not report it.
data "jamfplatform_security_cloud_uem_connect" "current" {}

output "last_sync" {
  value = try(data.jamfplatform_security_cloud_uem_connect.current.latest_sync.status, "never synced")
}
