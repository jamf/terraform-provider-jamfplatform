# Mac: look up Pages by its App Store ID. A Mac target reads the Mac listing,
# which has its own icon, description and version.
data "jamfplatform_apple_app_store_app" "pages_mac" {
  id     = "361309726"
  target = "mac"
}

# The provider downloads the icon from the App Store. When the developer changes
# the artwork, icon_url changes and the next plan replaces this icon.
resource "jamfplatform_pro_icon" "pages_mac" {
  icon_file_source = data.jamfplatform_apple_app_store_app.pages_mac.icon_url
}

resource "jamfplatform_pro_mac_app_store_app" "pages" {
  general = {
    name            = data.jamfplatform_apple_app_store_app.pages_mac.name
    version         = data.jamfplatform_apple_app_store_app.pages_mac.version
    bundle_id       = data.jamfplatform_apple_app_store_app.pages_mac.bundle_id
    url             = data.jamfplatform_apple_app_store_app.pages_mac.url
    is_free         = data.jamfplatform_apple_app_store_app.pages_mac.is_free
    deployment_type = "Make Available in Self Service"
  }

  self_service = {
    self_service_description = data.jamfplatform_apple_app_store_app.pages_mac.description
    self_service_icon = {
      id = jamfplatform_pro_icon.pages_mac.id
    }
  }

  # Each App Store release changes version. Ignore it here to keep releases out
  # of plans, or leave it managed to record every release as a planned change.
  lifecycle {
    ignore_changes = [general.version]
  }
}

# Mobile device: look up Keynote by bundle ID in the UK App Store.
data "jamfplatform_apple_app_store_app" "keynote_mobile" {
  bundle_id      = "com.apple.Keynote"
  target         = "mobile_device"
  country_region = "GB"
}

resource "jamfplatform_pro_icon" "keynote_mobile" {
  icon_file_source = data.jamfplatform_apple_app_store_app.keynote_mobile.icon_url
}

resource "jamfplatform_pro_mobile_device_app" "keynote" {
  general = {
    name                  = data.jamfplatform_apple_app_store_app.keynote_mobile.name
    version               = data.jamfplatform_apple_app_store_app.keynote_mobile.version
    bundle_id             = data.jamfplatform_apple_app_store_app.keynote_mobile.bundle_id
    itunes_store_url      = data.jamfplatform_apple_app_store_app.keynote_mobile.url
    itunes_country_region = data.jamfplatform_apple_app_store_app.keynote_mobile.country_region
    is_free               = data.jamfplatform_apple_app_store_app.keynote_mobile.is_free
  }

  self_service = {
    self_service_description = data.jamfplatform_apple_app_store_app.keynote_mobile.description
    self_service_icon = {
      id = jamfplatform_pro_icon.keynote_mobile.id
    }
  }

  lifecycle {
    ignore_changes = [general.version]
  }
}

# You can also look up an app by its store link. The provider uses the link's
# country or region when country_region is not set.
data "jamfplatform_apple_app_store_app" "from_link" {
  store_url = "https://apps.apple.com/us/app/xcode/id497799835?mt=12"
  target    = "mac"
}
