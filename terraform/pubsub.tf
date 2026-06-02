resource "google_pubsub_topic" "raw_activity" {
  name    = "topic-raw-activity"
  project = var.project_id

  # Enable message retention for replay (1 hour)
  message_retention_duration = "3600s"
}

# Mobile activity topic - carries mobile health activities from mobile-sync-handler
# mobile-source-handler subscribes to process into StandardizedActivity
resource "google_pubsub_topic" "mobile_activity" {
  name    = "topic-mobile-activity"
  project = var.project_id

  message_retention_duration = "3600s"
}

resource "google_pubsub_topic" "enriched_activity" {
  name    = "topic-enriched-activity"
  project = var.project_id

  # Enable message retention for replay (1 hour)
  message_retention_duration = "3600s"
}

# Pipeline activity topic - carries activities with specific pipelineId set
# Pipeline Splitter publishes here, Enricher subscribes
resource "google_pubsub_topic" "pipeline_activity" {
  name    = "topic-pipeline-activity"
  project = var.project_id

  # Enable message retention for replay (1 hour)
  message_retention_duration = "3600s"
}

resource "google_pubsub_topic" "destination_upload" {
  name    = "topic-destination-upload"
  project = var.project_id

  message_retention_duration = "3600s"
}

# Enrichment lag topic — receives activities that need to wait for device sync
# (e.g. Fitbit HR data not yet available). The enricher publishes here on first
# retry; this subscription pushes back to /pubsub/run with isLagRetry=true.
resource "google_pubsub_topic" "enrichment_lag" {
  name    = "topic-enrichment-lag"
  project = var.project_id

  # Retain for 2h — well beyond the 30-min lag window. If the pipeline service
  # is briefly down, lag messages survive and still get a chance to process.
  message_retention_duration = "7200s"
}

# Dead-letter topic for lag messages that exhaust all retries.
resource "google_pubsub_topic" "enrichment_lag_dead_letter" {
  name    = "topic-enrichment-lag-dead-letter"
  project = var.project_id

  message_retention_duration = "604800s" # 7 days for investigation
}

resource "google_pubsub_subscription" "enrichment_lag_sub" {
  name    = "sub-enrichment-lag"
  topic   = google_pubsub_topic.enrichment_lag.name
  project = var.project_id

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.backend["pipeline"].uri}/pubsub/run"
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa["pipeline"].email
    }
  }

  ack_deadline_seconds = 600

  # Retain for 2h, aligned to the topic retention.
  message_retention_duration = "7200s"

  retry_policy {
    minimum_backoff = "60s"
    maximum_backoff = "300s"
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.enrichment_lag_dead_letter.id
    max_delivery_attempts = 10
  }
}


resource "google_pubsub_topic" "parkrun_results_trigger" {
  name    = "topic-parkrun-results-trigger"
  project = var.project_id

  # Trigger messages are only useful until the next scheduled check (2h).
  # Short retention prevents stale triggers stacking up if the handler is down.
  message_retention_duration = "7200s"
}

resource "google_pubsub_subscription" "parkrun_results_check_sub" {
  name    = "sub-parkrun-results-check"
  topic   = google_pubsub_topic.parkrun_results_trigger.name
  project = var.project_id

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.backend["pipeline"].uri}/pubsub/parkrun-check"
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa["pipeline"].email
    }
  }

  # The checker scans all waiting parkrun pending inputs — give it 5 min.
  ack_deadline_seconds = 300

  # Discard undelivered messages after 2h (aligned to check interval).
  # If the handler is down that long, the next scheduler fire will start fresh.
  message_retention_duration = "7200s"

  retry_policy {
    minimum_backoff = "30s"
    maximum_backoff = "300s"
  }
}

# Registration summary topic - triggered daily by Cloud Scheduler
resource "google_pubsub_topic" "registration_summary_trigger" {
  name    = "topic-registration-summary-trigger"
  project = var.project_id
}

resource "google_pubsub_subscription" "destination_upload_sub" {
  name  = "sub-destination-upload"
  topic = google_pubsub_topic.destination_upload.name

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.backend["destination"].uri}/"
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa["destination"].email
    }
  }

  ack_deadline_seconds       = 600
  message_retention_duration = "3600s"

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }
}

resource "google_pubsub_subscription" "pipeline_raw_sub" {
  name  = "sub-pipeline-raw"
  topic = google_pubsub_topic.raw_activity.name

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.backend["pipeline"].uri}/pubsub/raw"
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa["pipeline"].email
    }
  }

  ack_deadline_seconds = 600
  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }
}

resource "google_pubsub_subscription" "pipeline_enriched_sub" {
  name  = "sub-pipeline-enriched"
  topic = google_pubsub_topic.enriched_activity.name

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.backend["pipeline"].uri}/pubsub/enriched"
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa["pipeline"].email
    }
  }

  ack_deadline_seconds = 600
  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }
}

resource "google_pubsub_subscription" "pipeline_run_sub" {
  name  = "sub-pipeline-run"
  topic = google_pubsub_topic.pipeline_activity.name

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.backend["pipeline"].uri}/pubsub/run"
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa["pipeline"].email
    }
  }

  ack_deadline_seconds = 600
  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }
}

# Roundup trigger — Cloud Scheduler publishes here to kick off roundup generation.
resource "google_pubsub_topic" "roundup_trigger" {
  name    = "topic-roundup-trigger"
  project = var.project_id

  message_retention_duration = "7200s"
}

resource "google_pubsub_subscription" "roundup_trigger_sub" {
  name    = "sub-roundup-trigger"
  topic   = google_pubsub_topic.roundup_trigger.name
  project = var.project_id

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.backend["activity"].uri}/pubsub/roundup"
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa["activity"].email
    }
  }

  ack_deadline_seconds       = 600
  message_retention_duration = "7200s"

  retry_policy {
    minimum_backoff = "60s"
    maximum_backoff = "600s"
  }
}

# Trial check trigger — Cloud Scheduler publishes here daily to sweep trial expirations.
resource "google_pubsub_topic" "trial_check_trigger" {
  name    = "topic-trial-check-trigger"
  project = var.project_id

  # Trigger messages are only useful until the next daily run. Drop after 26h.
  message_retention_duration = "93600s"
}

resource "google_pubsub_subscription" "trial_check_sub" {
  name    = "sub-trial-check"
  topic   = google_pubsub_topic.trial_check_trigger.name
  project = var.project_id

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.backend["billing"].uri}/pubsub/trial-check"
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa["billing"].email
    }
  }

  # Give the handler up to 5 min to sweep all trial users.
  ack_deadline_seconds = 300

  # Retain for 26h so a single missed run still gets delivered next day.
  message_retention_duration = "93600s"

  retry_policy {
    minimum_backoff = "60s"
    maximum_backoff = "300s"
  }
}

# Notification topic — any service publishes NotificationRequest messages here.
# The notification service fans out to whichever channels the user has enabled.
resource "google_pubsub_topic" "notification" {
  name    = "topic-notification"
  project = var.project_id

  # Retain for 1 hour; if the notification service is down briefly, messages are replayed.
  message_retention_duration = "3600s"
}

resource "google_pubsub_subscription" "notification_sub" {
  name    = "sub-notification"
  topic   = google_pubsub_topic.notification.name
  project = var.project_id

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.backend["notification"].uri}/pubsub/notifications"
    oidc_token {
      service_account_email = google_service_account.cloud_run_sa["notification"].email
    }
  }

  # Notifications are best-effort; 60 s is enough for FCM round-trips.
  ack_deadline_seconds       = 60
  message_retention_duration = "3600s"

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "60s"
  }
}
