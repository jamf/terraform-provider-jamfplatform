# Flush a policy's logs every time the policy changes. caller is the resource
# that fired the trigger, so caller.id is this policy's ID and the action needs
# no separate lookup. It resolves on create as well, once the ID exists.
resource "jamfplatform_pro_policy" "baseline" {
  general = {
    name      = "Baseline"
    enabled   = true
    frequency = "Once per computer"
  }

  scope = {
    targets = {
      computer_group_ids = [jamfplatform_device_group.pilot.jamf_pro_id]
    }
  }

  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.jamfplatform_pro_flush_policy_logs.reset_history]
    }
  }
}

resource "jamfplatform_device_group" "pilot" {
  name        = "Pilot Macs"
  group_type  = "static"
  device_type = "computer"
}

# quantity = "Zero" flushes every log, because no log is younger than zero days.
# There is no "Four" or "Five" quantity.
action "jamfplatform_pro_flush_policy_logs" "reset_history" {
  config {
    policy_id = caller.id
    quantity  = "Zero"
    period    = "Days"
  }
}

# Flush only logs older than six months, on demand for any policy:
#
#   terraform apply -invoke='action.jamfplatform_pro_flush_policy_logs.prune_old'
action "jamfplatform_pro_flush_policy_logs" "prune_old" {
  config {
    policy_id = jamfplatform_pro_policy.baseline.id
    quantity  = "Six"
    period    = "Months"
  }
}
