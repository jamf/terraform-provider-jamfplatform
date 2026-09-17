# Reach for jamfplatform_device_group unless the group has to belong to a Jamf
# Pro site. That is the only thing this resource adds.

# Criteria are evaluated in list order. Each entry joins to the one before it
# with its own and_or value, so the first entry's and_or is never used and
# moving an entry changes who is in the group.
resource "jamfplatform_pro_smart_computer_group" "recent_macbooks" {
  name        = "Recent MacBooks"
  description = "MacBook models running macOS 26"

  criteria = [
    {
      name        = "Model"
      search_type = "like"
      value       = "MacBook"
    },
    {
      name        = "Operating System Version"
      search_type = "like"
      value       = "26."
      and_or      = "and"
    },
  ]
}

# A group in a site only ever contains computers assigned to that same site.
resource "jamfplatform_pro_smart_computer_group" "eau_claire_filevault_pending" {
  name        = "Eau Claire - FileVault Not Enabled"
  description = "Site-scoped remediation target"
  site_id     = jamfplatform_pro_site.eau_claire.id

  criteria = [
    {
      name        = "FileVault 2 Status"
      search_type = "is not"
      value       = "All Partitions Encrypted"
    },
  ]
}

# Parentheses group the criteria. This matches a Mac that is either out of
# contact or missing its management framework, and is in the finance department.
resource "jamfplatform_pro_smart_computer_group" "finance_needs_attention" {
  name = "Finance - Needs Attention"

  criteria = [
    {
      name                    = "Last Check-in"
      search_type             = "more than x days ago"
      value                   = "30"
      has_opening_parenthesis = true
    },
    {
      name                    = "MDM Capability"
      search_type             = "is not"
      value                   = "Yes"
      and_or                  = "or"
      has_closing_parenthesis = true
    },
    {
      name        = "Department"
      search_type = "is"
      value       = "Finance"
      and_or      = "and"
    },
  ]
}

# A criterion naming a directory service group takes the group name; the
# provider resolves it against the configured directories. Paste the stored
# reference value instead when one name exists on more than one directory.
resource "jamfplatform_pro_smart_computer_group" "design_team_macs" {
  name = "Design Team Macs"

  criteria = [
    {
      name        = "Username directory service group"
      search_type = "member of"
      value       = "Design Team"
    },
  ]
}

resource "jamfplatform_pro_site" "eau_claire" {
  name = "Eau Claire"
}

# platform_id is what a Jamf Platform construct takes when it needs this group.
output "recent_macbooks_platform_id" {
  value = jamfplatform_pro_smart_computer_group.recent_macbooks.platform_id
}
