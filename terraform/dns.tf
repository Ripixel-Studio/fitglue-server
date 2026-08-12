locals {
  # Prod gets the root domain, others get subdomains
  dns_zone_name = var.environment == "prod" ? "fitglue-tech" : "${var.environment}-fitglue-tech"
  dns_name      = var.environment == "prod" ? "fitglue.tech." : "${var.environment}.fitglue.tech."

  # Base URL for OAuth redirects
  base_url = var.environment == "prod" ? "https://fitglue.tech" : "https://${var.environment}.fitglue.tech"
}

resource "google_dns_managed_zone" "main" {
  name        = local.dns_zone_name
  dns_name    = local.dns_name
  description = "Managed zone for ${local.dns_name}"
  visibility  = "public"

  labels = {
    managed-by  = "terraform"
    environment = var.environment
  }

  depends_on = [
    google_project_service.apis
  ]
}

# Delegation for Dev subdomain (only in Prod)
resource "google_dns_record_set" "dev_delegation" {
  count        = var.environment == "prod" ? 1 : 0
  managed_zone = google_dns_managed_zone.main.name
  project      = var.project_id
  name         = "dev.fitglue.tech."
  type         = "NS"
  ttl          = 300

  rrdatas = [
    "ns-cloud-d1.googledomains.com.",
    "ns-cloud-d2.googledomains.com.",
    "ns-cloud-d3.googledomains.com.",
    "ns-cloud-d4.googledomains.com.",
  ]
}


# Firebase Hosting Custom Domain
# Note: Firebase will provide DNS instructions after creation
# The custom domain resource will output the required DNS records
resource "google_firebase_hosting_custom_domain" "main" {
  provider      = google-beta
  project       = var.project_id
  site_id       = var.project_id
  custom_domain = var.domain_name

  # Don't wait for DNS verification in Terraform
  # We'll configure DNS manually based on Firebase's instructions
  wait_dns_verification = false

  depends_on = [
    google_dns_managed_zone.main
  ]
}
# DNS Records for Firebase Hosting
# All environments are zone apexes, so they must use A records (not CNAME)

# A record for Firebase Hosting (all environments)
resource "google_dns_record_set" "firebase_a" {
  managed_zone = google_dns_managed_zone.main.name
  name         = "${var.domain_name}."
  type         = "A"
  ttl          = 300
  rrdatas      = ["199.36.158.100"]
}

# TXT records for domain verification + email authentication (all environments)
# Cloud DNS requires a single record set per name+type, so SPF must be merged here
resource "google_dns_record_set" "firebase_txt" {
  managed_zone = google_dns_managed_zone.main.name
  name         = "${var.domain_name}."
  type         = "TXT"
  ttl          = 300
  rrdatas = [
    "\"hosting-site=${var.project_id}\"",
    "\"v=spf1 include:_spf.google.com ~all\"",
  ]
}

# Assets subdomain A record - points to Firebase Hosting, which 301s every
# path to the public showcase-assets GCS bucket (see firebase.tf). Previously
# pointed at the showcase-assets global LB; 199.36.158.100 is Firebase
# Hosting's dedicated custom-domain IP.
# Domain pattern: dev -> assets.dev.fitglue.tech, test -> assets.test.fitglue.tech, prod -> assets.fitglue.tech
resource "google_dns_record_set" "assets_a" {
  managed_zone = google_dns_managed_zone.main.name
  name         = var.environment == "prod" ? "assets.fitglue.tech." : "assets.${var.domain_name}."
  type         = "A"
  ttl          = 300
  rrdatas      = ["199.36.158.100"]
}

# DMARC record for email authentication
resource "google_dns_record_set" "dmarc" {
  managed_zone = google_dns_managed_zone.main.name
  name         = "_dmarc.${var.domain_name}."
  type         = "TXT"
  ttl          = 300
  rrdatas      = ["\"v=DMARC1; p=none; rua=mailto:system@fitglue.tech\""]
}

