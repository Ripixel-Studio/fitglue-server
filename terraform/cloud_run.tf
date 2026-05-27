locals {
  frontend_services = {
    "api-client"  = { is_public = true }
    "api-admin"   = { is_public = true }
    "api-public"  = { is_public = true }
    "api-webhook" = { is_public = true }
  }
  backend_services = {
    "user"        = { is_public = false }
    "billing"     = { is_public = false }
    "pipeline"    = { is_public = false }
    "activity"    = { is_public = false }
    "registry"    = { is_public = false }
    "destination" = { is_public = false }
  }
  all_services = merge(local.frontend_services, local.backend_services)
}

resource "google_artifact_registry_repository" "services" {
  location      = var.region
  repository_id = "fitglue-services"
  description   = "Docker repository for FitGlue Cloud Run services"
  format        = "DOCKER"
}

resource "google_service_account" "cloud_run_sa" {
  for_each     = local.all_services
  account_id   = "cr-${each.key}-sa"
  display_name = "Cloud Run Service Account for ${each.key}"
}

resource "google_cloud_run_v2_service" "backend" {
  for_each            = local.backend_services
  name                = each.key
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = false

  template {
    service_account = google_service_account.cloud_run_sa[each.key].email
    containers {
      image = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.services.name}/${each.key}:${var.image_tag}"
      resources {
        limits = {
          cpu    = "1000m"
          memory = "512Mi"
        }
      }

      # ── Shared env vars (all backend services) ──
      env {
        name  = "PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "GOOGLE_CLOUD_PROJECT"
        value = var.project_id
      }
      env {
        name  = "ENVIRONMENT"
        value = var.environment
      }
      env {
        name  = "LOG_LEVEL"
        value = var.log_level
      }
      env {
        name  = "SENTRY_DSN"
        value = var.sentry_dsn
      }

      # ── Pipeline-specific env vars ──
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name  = "DESTINATION_SERVICE_URL"
          value = "https://destination-${data.google_project.project.number}.${var.region}.run.app"
        }
      }
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name  = "GCS_ARTIFACT_BUCKET"
          value = "${var.project_id}-artifacts"
        }
      }
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name  = "ARTIFACT_BUCKET"
          value = "${var.project_id}-artifacts"
        }
      }
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name  = "SHOWCASE_ASSETS_BUCKET"
          value = google_storage_bucket.showcase_assets_bucket.name
        }
      }
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name  = "ASSETS_BASE_URL"
          value = "https://assets.${var.domain_name}"
        }
      }
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name  = "USER_SERVICE_URL"
          value = google_cloud_run_v2_service.backend["user"].uri
        }
      }

      # ── User service env vars ──
      dynamic "env" {
        for_each = each.key == "user" ? [1] : []
        content {
          name  = "FITGLUE_WEB_URL"
          value = var.base_url
        }
      }
      dynamic "env" {
        for_each = each.key == "user" ? [1] : []
        content {
          name  = "BASE_URL"
          value = var.base_url
        }
      }
      dynamic "env" {
        for_each = each.key == "user" ? [1] : []
        content {
          name  = "SYSTEM_EMAIL"
          value = "system@fitglue.tech"
        }
      }

      # ── Activity service env vars ──
      dynamic "env" {
        for_each = each.key == "activity" ? [1] : []
        content {
          name  = "SHOWCASE_ASSETS_BUCKET"
          value = google_storage_bucket.showcase_assets_bucket.name
        }
      }

      # ── Destination service env vars ──
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name  = "GCS_ARTIFACT_BUCKET"
          value = "${var.project_id}-artifacts"
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name  = "USER_SERVICE_URL"
          value = "https://user-${data.google_project.project.number}.${var.region}.run.app"
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name  = "ACTIVITY_SERVICE_URL"
          value = "https://activity-${data.google_project.project.number}.${var.region}.run.app"
        }
      }

      # ═══════════════════════════════════════════════════════════════
      # Secrets (from Secret Manager)
      # ═══════════════════════════════════════════════════════════════

      # ── Pipeline secrets (enricher needs Gemini, Spotify, Fitbit) ──
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name = "GEMINI_API_KEY"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.gemini_api_key.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name = "SPOTIFY_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.spotify_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name = "SPOTIFY_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.spotify_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name = "FITBIT_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.fitbit_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "pipeline" ? [1] : []
        content {
          name = "FITBIT_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.fitbit_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }

      # ── Billing secrets (Stripe) ──
      dynamic "env" {
        for_each = each.key == "billing" ? [1] : []
        content {
          name = "STRIPE_SECRET_KEY"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.stripe_secret_key.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "billing" ? [1] : []
        content {
          name = "STRIPE_WEBHOOK_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.stripe_webhook_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "billing" ? [1] : []
        content {
          name = "STRIPE_PRICE_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.stripe_price_id.secret_id
              version = "latest"
            }
          }
        }
      }

      # ── User secrets (Email) ──
      dynamic "env" {
        for_each = each.key == "user" ? [1] : []
        content {
          name = "EMAIL_APP_PASSWORD"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.email_app_password.secret_id
              version = "latest"
            }
          }
        }
      }

      # ── Destination secrets (all OAuth client pairs for token refresh) ──
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "STRAVA_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.strava_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "STRAVA_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.strava_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "FITBIT_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.fitbit_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "FITBIT_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.fitbit_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "TRAININGPEAKS_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.trainingpeaks_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "TRAININGPEAKS_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.trainingpeaks_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "GOOGLE_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.google_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "GOOGLE_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.google_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "GITHUB_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.github_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "GITHUB_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.github_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "SPOTIFY_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.spotify_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "SPOTIFY_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.spotify_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "WAHOO_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.wahoo_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "WAHOO_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.wahoo_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "POLAR_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.polar_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "POLAR_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.polar_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "OURA_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.oura_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "destination" ? [1] : []
        content {
          name = "OURA_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.oura_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
    }
    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }
  }
}



resource "google_cloud_run_v2_service" "frontend" {
  for_each            = local.frontend_services
  name                = each.key
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_ALL"
  deletion_protection = false

  template {
    service_account = google_service_account.cloud_run_sa[each.key].email
    containers {
      image = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.services.name}/${each.key}:${var.image_tag}"
      resources {
        limits = {
          cpu    = "1000m"
          memory = "512Mi"
        }
      }

      # ── Shared env vars (all frontend services) ──
      env {
        name  = "PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "GOOGLE_CLOUD_PROJECT"
        value = var.project_id
      }
      env {
        name  = "ENVIRONMENT"
        value = var.environment
      }
      env {
        name  = "LOG_LEVEL"
        value = var.log_level
      }
      env {
        name  = "SENTRY_DSN"
        value = var.sentry_dsn
      }
      env {
        name  = "USER_SERVICE_URL"
        value = google_cloud_run_v2_service.backend["user"].uri
      }
      env {
        name  = "BILLING_SERVICE_URL"
        value = google_cloud_run_v2_service.backend["billing"].uri
      }
      env {
        name  = "PIPELINE_SERVICE_URL"
        value = google_cloud_run_v2_service.backend["pipeline"].uri
      }
      env {
        name  = "ACTIVITY_SERVICE_URL"
        value = google_cloud_run_v2_service.backend["activity"].uri
      }
      env {
        name  = "REGISTRY_SERVICE_URL"
        value = google_cloud_run_v2_service.backend["registry"].uri
      }

      # ── api-client: OAuth base URLs ──
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name  = "API_URL"
          value = var.base_url
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name  = "WEB_URL"
          value = var.base_url
        }
      }

      # ═══════════════════════════════════════════════════════════════
      # Secrets — api-client (all OAuth client pairs for connect flow)
      # ═══════════════════════════════════════════════════════════════
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "OAUTH_STATE_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.oauth_state_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "STRAVA_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.strava_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "STRAVA_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.strava_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "FITBIT_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.fitbit_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "FITBIT_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.fitbit_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "OURA_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.oura_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "OURA_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.oura_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "POLAR_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.polar_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "POLAR_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.polar_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "WAHOO_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.wahoo_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "WAHOO_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.wahoo_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "SPOTIFY_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.spotify_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "SPOTIFY_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.spotify_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "GITHUB_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.github_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "GITHUB_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.github_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "GOOGLE_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.google_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "GOOGLE_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.google_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "TRAININGPEAKS_CLIENT_ID"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.trainingpeaks_client_id.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-client" ? [1] : []
        content {
          name = "TRAININGPEAKS_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.trainingpeaks_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }

      # ═══════════════════════════════════════════════════════════════
      # Secrets — api-webhook (webhook verification tokens + client pairs used in fetch)
      # ═══════════════════════════════════════════════════════════════
      dynamic "env" {
        for_each = each.key == "api-webhook" ? [1] : []
        content {
          name = "STRAVA_WEBHOOK_VERIFY_TOKEN"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.strava_verify_token.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-webhook" ? [1] : []
        content {
          name = "FITBIT_SUBSCRIBER_VERIFICATION_TOKEN"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.fitbit_verification_code.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-webhook" ? [1] : []
        content {
          name = "GITHUB_WEBHOOK_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.github_webhook_secret.secret_id
              version = "latest"
            }
          }
        }
      }
      dynamic "env" {
        for_each = each.key == "api-webhook" ? [1] : []
        content {
          name = "FITBIT_OAUTH_CLIENT_SECRET"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.fitbit_client_secret.secret_id
              version = "latest"
            }
          }
        }
      }
    }
    scaling {
      min_instance_count = 0
      max_instance_count = 3
    }
  }
}

data "google_iam_policy" "noauth" {
  binding {
    role = "roles/run.invoker"
    members = [
      "allUsers",
    ]
  }
}

resource "google_cloud_run_service_iam_policy" "public_access" {
  for_each    = local.frontend_services
  location    = google_cloud_run_v2_service.frontend[each.key].location
  project     = google_cloud_run_v2_service.frontend[each.key].project
  service     = google_cloud_run_v2_service.frontend[each.key].name
  policy_data = data.google_iam_policy.noauth.policy_data
}



locals {
  backend_invokers = flatten([
    for be_k, be_v in local.backend_services : [
      for fe_k, fe_v in local.frontend_services : {
        backend  = be_k
        frontend = fe_k
      }
    ]
  ])
}

resource "google_cloud_run_v2_service_iam_member" "internal_invocation" {
  for_each = {
    for pair in local.backend_invokers : "${pair.backend}-${pair.frontend}" => pair
  }
  name     = google_cloud_run_v2_service.backend[each.value.backend].name
  location = google_cloud_run_v2_service.backend[each.value.backend].location
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.cloud_run_sa[each.value.frontend].email}"
}

# Pipeline also invokes Destination
resource "google_cloud_run_v2_service_iam_member" "pipeline_to_destination" {
  name     = google_cloud_run_v2_service.backend["destination"].name
  location = google_cloud_run_v2_service.backend["destination"].location
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.cloud_run_sa["pipeline"].email}"
}

# Destination invokes User (GetProfile, ListIntegrations)
resource "google_cloud_run_v2_service_iam_member" "destination_to_user" {
  name     = google_cloud_run_v2_service.backend["user"].name
  location = google_cloud_run_v2_service.backend["user"].location
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.cloud_run_sa["destination"].email}"
}

# Destination invokes Activity (showcase uploader uses activity client)
resource "google_cloud_run_v2_service_iam_member" "destination_to_activity" {
  name     = google_cloud_run_v2_service.backend["activity"].name
  location = google_cloud_run_v2_service.backend["activity"].location
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.cloud_run_sa["destination"].email}"
}

# Pub/Sub push subscriptions use OIDC tokens with each service's own SA,
# so the SA needs run.invoker on itself to authenticate the push.
locals {
  pubsub_push_targets = ["pipeline", "destination"]
}

resource "google_cloud_run_v2_service_iam_member" "pubsub_self_invoke" {
  for_each = toset(local.pubsub_push_targets)
  name     = google_cloud_run_v2_service.backend[each.key].name
  location = google_cloud_run_v2_service.backend[each.key].location
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.cloud_run_sa[each.key].email}"
}
