resource "google_firebase_web_app" "web" {
  provider     = google-beta
  project      = var.project_id
  display_name = "fitglue-web-app"
}

data "google_firebase_web_app_config" "web" {
  provider   = google-beta
  web_app_id = google_firebase_web_app.web.app_id
}

resource "google_firebase_hosting_site" "default" {
  provider = google-beta
  project  = var.project_id
  site_id  = var.project_id # Default site
  app_id   = google_firebase_web_app.web.app_id
}

# ── Showcase assets via Firebase Hosting redirect ──
# Replaces the global HTTPS LB + Cloud CDN that fronted the showcase assets
# bucket (~$18/mo/env for the forwarding rules alone). The bucket is public,
# so assets.<domain>/<path> can simply 301 to storage.googleapis.com — every
# historical URL baked into showcase metadata keeps working, at zero standing
# cost. The LB in cdn.tf is removed in a follow-up once the cert here is live.
resource "google_firebase_hosting_site" "assets" {
  provider = google-beta
  project  = var.project_id
  site_id  = "${var.project_id}-assets"
}

resource "google_firebase_hosting_version" "assets_redirect" {
  provider = google-beta
  site_id  = google_firebase_hosting_site.assets.site_id

  config {
    redirects {
      glob        = "/:path*"
      status_code = 301
      location    = "https://storage.googleapis.com/${google_storage_bucket.showcase_assets_bucket.name}/:path"
    }
  }
}

resource "google_firebase_hosting_release" "assets_redirect" {
  provider     = google-beta
  site_id      = google_firebase_hosting_site.assets.site_id
  version_name = google_firebase_hosting_version.assets_redirect.name
  message      = "Redirect all asset paths to the public GCS bucket"
}

resource "google_firebase_hosting_custom_domain" "assets" {
  provider = google-beta
  project  = var.project_id
  site_id  = google_firebase_hosting_site.assets.site_id
  custom_domain = var.environment == "prod" ? "assets.fitglue.tech" : "assets.${var.environment}.fitglue.tech"

  # Don't block the apply on DNS/cert propagation; the DNS record below is
  # created in the same apply and the cert issues asynchronously.
  wait_dns_verification = false
}
