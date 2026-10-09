terraform {
  required_providers {
    jamfplatform = {
      source = "jamf/jamfplatform"
    }
  }
}

provider "jamfplatform" {
  base_url      = "https://us.api.jamfcloud.com" # or "https://eu.api.jamfcloud.com", "https://apac.api.jamfcloud.com"
  client_id     = "11111111-1111-4111-8111-111111111111"
  client_secret = "example-client-secret"
  # The "Platform environment" your API integration targets. This is the
  # preferred scope for new integrations.
  environment_id = "22222222-2222-4222-8222-222222222222"

  # Legacy alternative: the single "Tenant" your API integration targets. Set
  # exactly one of environment_id or tenant_id, and set the one your integration
  # was actually created for.
  # tenant_id = "22222222-2222-4222-8222-222222222222"

  # Only needed when traffic reaches the Platform API through a reverse proxy
  # that authenticates callers itself. An ordinary forward proxy needs nothing
  # beyond HTTPS_PROXY / NO_PROXY in the environment. See the reverse proxy
  # guide.
  #
  # custom_headers = {
  #   "X-Proxy-Route" = "eu-west"
  #   "Authorization" = "Basic ${var.proxy_basic_credential}"
  # }
  #
  # Send the Platform API credential in this header instead of Authorization,
  # leaving Authorization free for the proxy's own credential above.
  # authorization_header_name = "X-Jamf-Authorization"
}
