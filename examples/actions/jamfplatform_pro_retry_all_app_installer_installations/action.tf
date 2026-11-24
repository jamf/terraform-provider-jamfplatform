# Run on demand:
#
#   terraform apply -invoke='action.jamfplatform_pro_retry_all_app_installer_installations.retry_all'

# Retries every failed App Installer installation in the tenant, across all
# deployments. Scope one deployment with
# jamfplatform_pro_retry_app_installer_installations instead.
action "jamfplatform_pro_retry_all_app_installer_installations" "retry_all" {
  config {}
}

# Fires the retry once a day. The date string changes only when the calendar
# day does, where timestamp() alone would change on every apply and retry the
# whole tenant each time.
resource "terraform_data" "nightly_retry" {
  input = formatdate("YYYY-MM-DD", timestamp())

  lifecycle {
    action_trigger {
      events  = [after_update]
      actions = [action.jamfplatform_pro_retry_all_app_installer_installations.retry_all]
    }
  }
}
