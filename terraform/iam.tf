locals {
  firestore_services = ["user", "billing", "pipeline", "activity", "registry", "api-admin", "destination", "api-client", "notification", "api-webhook"]
  pubsub_publishers  = ["api-webhook", "pipeline", "activity", "api-client", "destination"]
  secret_accessors   = ["api-client", "user", "billing", "pipeline", "activity", "destination", "registry", "api-webhook"]
  storage_services   = ["activity", "pipeline", "destination"]
}

resource "google_project_iam_member" "cr_firestore_user" {
  for_each = toset(local.firestore_services)
  project  = var.project_id
  role     = "roles/datastore.user"
  member   = "serviceAccount:${google_service_account.cloud_run_sa[each.key].email}"
}

resource "google_project_iam_member" "cr_pubsub_publisher" {
  for_each = toset(local.pubsub_publishers)
  project  = var.project_id
  role     = "roles/pubsub.publisher"
  member   = "serviceAccount:${google_service_account.cloud_run_sa[each.key].email}"
}

resource "google_project_iam_member" "cr_secret_accessor" {
  for_each = toset(local.secret_accessors)
  project  = var.project_id
  role     = "roles/secretmanager.secretAccessor"
  member   = "serviceAccount:${google_service_account.cloud_run_sa[each.key].email}"
}

resource "google_project_iam_member" "cr_storage_admin" {
  for_each = toset(local.storage_services)
  project  = var.project_id
  role     = "roles/storage.objectAdmin"
  member   = "serviceAccount:${google_service_account.cloud_run_sa[each.key].email}"
}

resource "google_project_iam_member" "cr_fcm_admin" {
  project = var.project_id
  role    = "roles/firebasecloudmessaging.admin"
  member  = "serviceAccount:${google_service_account.cloud_run_sa["notification"].email}"
}

# Firebase Auth Admin - allows get/delete user
resource "google_project_iam_member" "cr_firebase_auth_admin" {
  project = var.project_id
  role    = "roles/firebaseauth.admin"
  member  = "serviceAccount:${google_service_account.cloud_run_sa["user"].email}"
}

# AI Platform User - allows enricher to call Vertex AI Imagen API
resource "google_project_iam_member" "cr_aiplatform_user" {
  project = var.project_id
  role    = "roles/aiplatform.user"
  member  = "serviceAccount:${google_service_account.cloud_run_sa["pipeline"].email}"
}

# Service Account Token Creator - allows getSignedUrl() for GCS objects
resource "google_project_iam_member" "cr_token_creator" {
  project = var.project_id
  role    = "roles/iam.serviceAccountTokenCreator"
  member  = "serviceAccount:${google_service_account.cloud_run_sa["activity"].email}"
}

resource "google_project_iam_member" "web_deployer_run_viewer" {
  project = var.project_id
  role    = "roles/run.viewer"
  member  = "serviceAccount:circleci-web-deployer@${var.project_id}.iam.gserviceaccount.com"
}

# Service Usage Consumer - allows Firebase CLI to check API enablement status during deploy
resource "google_project_iam_member" "web_deployer_service_usage_consumer" {
  project = var.project_id
  role    = "roles/serviceusage.serviceUsageConsumer"
  member  = "serviceAccount:circleci-web-deployer@${var.project_id}.iam.gserviceaccount.com"
}

# Firebase Rules Admin - allows Firebase CLI to validate and deploy Firestore rules
resource "google_project_iam_member" "web_deployer_firebase_rules_admin" {
  project = var.project_id
  role    = "roles/firebaserules.admin"
  member  = "serviceAccount:circleci-web-deployer@${var.project_id}.iam.gserviceaccount.com"
}

# Pub/Sub service agent needs publisher and subscriber rights to operate dead-letter
# forwarding. Using project-level bindings to match the deployer's IAM scope
# (resource-level pubsub IAM requires pubsub.admin which the deployer doesn't have).
resource "google_project_iam_member" "pubsub_sa_publisher" {
  project = var.project_id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_project_iam_member" "pubsub_sa_subscriber" {
  project = var.project_id
  role    = "roles/pubsub.subscriber"
  member  = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
