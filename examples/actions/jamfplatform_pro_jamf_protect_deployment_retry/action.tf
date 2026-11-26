# Each block below is one way to pick the computers. Invoke the one you want by
# name, for example:
#
#   terraform apply -invoke='action.jamfplatform_pro_jamf_protect_deployment_retry.all_failed'

# Retry failed Jamf Protect install tasks for a deployment.
#
# The deployment id is the plan UUID surfaced by the plans data source.
data "jamfplatform_pro_jamf_protect_plans" "all" {}

locals {
  # Pick the deployment (plan) whose profile you want to retry.
  deployment_id = data.jamfplatform_pro_jamf_protect_plans.all.plans[0].uuid
}

# Mode 1: retry one computer's failed task(s), by serial number.
action "jamfplatform_pro_jamf_protect_deployment_retry" "by_serial" {
  config {
    deployment_id = local.deployment_id
    serial_number = "C02XXXXXXXXX"
  }
}

# Mode 2: retry one computer's failed task(s), by management id.
action "jamfplatform_pro_jamf_protect_deployment_retry" "by_management_id" {
  config {
    deployment_id = local.deployment_id
    management_id = "0d1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8"
  }
}

# Mode 2b: retry one computer's failed task(s), by hardware UDID.
action "jamfplatform_pro_jamf_protect_deployment_retry" "by_udid" {
  config {
    deployment_id = local.deployment_id
    udid          = "00000000-0000-0000-0000-000000000000"
  }
}

# Mode 3: retry every failed task in the deployment (UI "Retry Failed").
action "jamfplatform_pro_jamf_protect_deployment_retry" "all_failed" {
  config {
    deployment_id = local.deployment_id
    all_failed    = true
  }
}

# Mode 4: retry an explicit set of deployment task ids (advanced escape hatch).
action "jamfplatform_pro_jamf_protect_deployment_retry" "by_task_ids" {
  config {
    deployment_id = local.deployment_id
    task_ids      = ["82", "83"]
  }
}
