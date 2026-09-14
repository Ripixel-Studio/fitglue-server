resource "google_storage_bucket" "artifacts_bucket" {
  name     = "${var.project_id}-artifacts"
  location = var.region

  uniform_bucket_level_access = true

  # Raw provider payloads (payloads/ prefix) are pruned after 30 days; everything
  # else — FIT files (activities/), enriched events (enriched_events/) and the durable
  # layered activity records (activity_records/) — persists indefinitely.
  #
  # This rule is deliberately PREFIX-SCOPED. A previous unscoped 7-day delete rule
  # silently destroyed all pre-2026-07-17 showcase detail and made activity detail 404
  # after a week (issue #34); matches_prefix confines deletion to the raw payloads that
  # layered storage now makes disposable. The parsed source and enricher outputs are
  # kept in the activity_records/ blob (no TTL), so pruning the raw payload only means
  # reprocessing an activity older than 30 days relies on a re-pull from the source
  # (editable-activities spec, DECISION 1a).
  lifecycle_rule {
    condition {
      age            = 30
      matches_prefix = ["payloads/"]
    }
    action {
      type = "Delete"
    }
  }

  cors {
    origin          = [var.base_url]
    method          = ["PUT", "POST", "GET", "HEAD"]
    response_header = ["Content-Type", "Content-Length", "x-goog-content-length-range"]
    max_age_seconds = 3600
  }
}

# The api-client gateway generates V4 signed download URLs for individual pipeline-run
# payloads (GET /users/me/pipeline-runs/{runId}/payload). Signing only needs Token Creator,
# but GCS also validates that the *signing identity* can read the object — so the api-client
# SA must hold an object-read role on this bucket, otherwise every signed GET returns 403.
# (The bundle export is signed by the activity SA, which already has project-level objectAdmin.)
resource "google_storage_bucket_iam_member" "artifacts_api_client_read" {
  bucket = google_storage_bucket.artifacts_bucket.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.cloud_run_sa["api-client"].email}"
}

# Version config bucket - stores unified FitGlue version across web/server repos
resource "google_storage_bucket" "version_config_bucket" {
  name     = "${var.project_id}-version-config"
  location = var.region

  uniform_bucket_level_access = true

  versioning {
    enabled = true
  }
}

# Grant CI service account access to read/write version config
resource "google_storage_bucket_iam_member" "version_config_ci_access" {
  bucket = google_storage_bucket.version_config_bucket.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:circleci-deployer@${var.project_id}.iam.gserviceaccount.com"
}

# Grant Web CI service account read access to version config
resource "google_storage_bucket_iam_member" "version_config_web_ci_access" {
  bucket = google_storage_bucket.version_config_bucket.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:circleci-web-deployer@${var.project_id}.iam.gserviceaccount.com"
}

# Showcase assets bucket - stores generated images (AI banners, route thumbnails, muscle heatmaps)
resource "google_storage_bucket" "showcase_assets_bucket" {
  name     = "${var.project_id}-showcase-assets"
  location = var.region

  uniform_bucket_level_access = true

  # No lifecycle rule - showcase assets are referenced by Showcase pages forever

  cors {
    origin          = [var.base_url, "*"]
    method          = ["GET", "HEAD", "PUT"]
    response_header = ["Content-Type", "Content-Length", "x-goog-content-length-range"]
    max_age_seconds = 3600
  }
}

# Grant public read access to showcase assets (images are public for Showcase viewing)
resource "google_storage_bucket_iam_member" "showcase_assets_public_read" {
  bucket = google_storage_bucket.showcase_assets_bucket.name
  role   = "roles/storage.objectViewer"
  member = "allUsers"
}

# Grant Cloud Functions service account access to write showcase assets
resource "google_storage_bucket_iam_member" "showcase_assets_functions_write" {
  bucket = google_storage_bucket.showcase_assets_bucket.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${var.project_id}@appspot.gserviceaccount.com"
}

# NOTE: The Artifact Registry "cloud-run-source-deploy" is managed manually, not via Terraform.
# Create it for each environment with:
#   gcloud artifacts repositories create cloud-run-source-deploy \
#     --repository-format=docker --location=us-central1 \
#     --project=fitglue-server-<env>
