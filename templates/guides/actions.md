---
page_title: "Using actions"
description: |-
  Run Jamf device commands and maintenance tasks from Terraform with actions: on demand with -invoke, or from a resource's lifecycle.
---

# Using actions

An action runs a one-off task: send a push, flush logs, lock a Mac. It has no state, so it is not
a resource. Declaring one changes nothing. It runs when you ask for it, in one of two ways.

Actions need Terraform 1.14 or later.

## Run it on demand

Name the action on the command line:

```shell
terraform apply -invoke='action.jamfplatform_pro_send_blank_push.nudge'
```

Terraform plans and runs that action alone. Use this for commands you choose to send, such as
an erase, a lock or a log flush on a policy you did not change.

## Run it from a resource

Add an `action_trigger` to a resource's `lifecycle`. The action runs after the resource is
created, updated, or both:

```terraform
resource "jamfplatform_pro_policy" "baseline" {
  general = {
    name      = "Baseline"
    enabled   = true
    frequency = "Once per computer"
  }

  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.jamfplatform_pro_flush_policy_logs.reset_history]
    }
  }
}

action "jamfplatform_pro_flush_policy_logs" "reset_history" {
  config {
    policy_id = caller.id
    quantity  = "Zero"
    period    = "Days"
  }
}
```

`caller` is the resource that fired the trigger, so `caller.id` is the policy's ID and the action
needs no second reference to it. Any attribute of that resource works: `caller.members` on a
static device group, `caller.jamf_pro_id` where an action wants a numeric Jamf Pro ID. A trigger
on a resource that is being created resolves once the resource exists.

The plan lists each action under the change that triggers it:

```text
Actions to be invoked after this change in order:
# action.jamfplatform_pro_flush_policy_logs.reset_history
...
Plan: 0 to add, 1 to change, 0 to destroy. Actions: 1 to invoke.
```

An `after_update` trigger fires on an apply that changes the resource. If nothing changes,
nothing runs.

## Choosing between them

| Use `-invoke` | Use `action_trigger` |
|---|---|
| Destructive or disruptive commands: erase, unmanage, lock, shut down | Follow-up that belongs to a change: flush a policy's logs, nudge a group's members |
| Break-glass repairs | Work you want repeated every time the resource changes |
| One-off operations on a device you look up by serial number | Anything that should be visible in the plan review |

A trigger repeats on every matching event, so keep it off a resource that changes often unless the
action is safe to repeat.

## Targets

Device actions take their targets as serial numbers, Jamf management IDs or, for a group, the
group's members. Source them from a data source rather than typing them:

```terraform
data "jamfplatform_devices" "macs" {
  filter = [
    {
      selector = "modelIdentifier"
      argument = "Mac*"
    }
  ]
}

action "jamfplatform_pro_send_blank_push" "nudge" {
  config {
    management_ids = [for d in data.jamfplatform_devices.macs.devices : d.id]
  }
}
```

## Timeouts

Every action takes a `timeouts` block with an `invoke` value, a duration such as `30s` or `10m`.
It defaults to five minutes.

```terraform
action "jamfplatform_pro_send_blank_push" "nudge" {
  config {
    management_ids = caller.members
    timeouts = {
      invoke = "2m"
    }
  }
}
```

## Results

An action reports progress in the apply output and fails the apply if Jamf refuses the request. It
returns no value, so nothing downstream can read what it did. Where an outcome is readable
elsewhere, such as a sync status on a data source, the action's own page says so.
