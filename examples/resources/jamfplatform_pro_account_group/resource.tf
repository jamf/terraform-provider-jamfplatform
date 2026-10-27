# An account group with an explicit privilege grid. Jamf Pro accepts only the
# Custom privilege set from an API client, which is how this provider
# authenticates, so every group uses it. Privileges are grouped
# by the same categories as the Jamf Pro admin UI Privileges tab. Jamf Pro
# silently adds dependency privileges and silently ignores unrecognised ones;
# the provider validates declared privileges at plan time against the tenant's
# Administrator catalog and reconciles server-added extras out of state.
resource "jamfplatform_pro_account_group" "helpdesk" {
  display_name  = "Help Desk"
  access_level  = "Full Access"
  privilege_set = "Custom"

  privileges = {
    jamf_pro_server_objects = [
      "Read Computers",
      "Read Mobile Devices",
      "Read Users",
    ]
    jamf_pro_server_actions = [
      "Send Computer Remote Lock Command",
    ]
  }
}

# A directory-backed group whose membership is sourced from an LDAP / cloud
# identity provider. Reference the backing server by ID and leave `members`
# unset so the directory manages membership.
resource "jamfplatform_pro_account_group" "directory_admins" {
  display_name   = "Directory Admins"
  access_level   = "Full Access"
  privilege_set  = "Custom"
  ldap_server_id = 31

  privileges = {
    jamf_pro_server_objects = ["Read Computers"]
  }
}

# A group with explicitly-managed Jamf-Pro-local members. Reference administrator
# account IDs (see jamfplatform_pro_account). This is the authoritative side of
# account-to-group membership.
resource "jamfplatform_pro_account_group" "managed_members" {
  display_name  = "Managed Members"
  access_level  = "Full Access"
  privilege_set = "Custom"
  members       = [12, 56]

  privileges = {
    jamf_pro_server_objects = ["Read Computers"]
  }
}
