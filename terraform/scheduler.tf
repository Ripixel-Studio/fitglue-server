# Cloud Scheduler jobs
#
# Retry rules for the parkrun results check:
#   - Fires 6x/day at 10:00, 12:00, 14:00, 16:00, 18:00, 20:00 Europe/London.
#     This covers the typical parkrun results window (published ~noon to evening)
#     without polling overnight. Over the 48h AutoDeadline, max 12 attempts.
#   - On transient handler failure: Pub/Sub retries within the 2h message retention
#     window (sub-parkrun-results-check), then drops. The next scheduler fire
#     starts a clean run — no stacking.
#   - Pending inputs past AutoDeadline are marked COMPLETED by the handler itself,
#     so they stop appearing in future runs.

# Roundup generation jobs — fire at period boundaries.
resource "google_cloud_scheduler_job" "weekly_roundup_generator" {
  name        = "weekly-roundup-generator"
  description = "Triggers showcase roundup generation for the week that just ended"
  project     = var.project_id
  region      = var.region
  schedule    = "0 6 * * 1"
  time_zone   = "UTC"

  pubsub_target {
    topic_name = google_pubsub_topic.roundup_trigger.id
    data       = base64encode(jsonencode({ period_type = "week" }))
  }

  retry_config {
    retry_count = 1
  }
}

resource "google_cloud_scheduler_job" "monthly_roundup_generator" {
  name        = "monthly-roundup-generator"
  description = "Triggers showcase roundup generation for the month that just ended"
  project     = var.project_id
  region      = var.region
  schedule    = "0 6 1 * *"
  time_zone   = "UTC"

  pubsub_target {
    topic_name = google_pubsub_topic.roundup_trigger.id
    data       = base64encode(jsonencode({ period_type = "month" }))
  }

  retry_config {
    retry_count = 1
  }
}

resource "google_cloud_scheduler_job" "yearly_roundup_generator" {
  name        = "yearly-roundup-generator"
  description = "Triggers showcase roundup generation for the year that just ended"
  project     = var.project_id
  region      = var.region
  schedule    = "0 6 1 1 *"
  time_zone   = "UTC"

  pubsub_target {
    topic_name = google_pubsub_topic.roundup_trigger.id
    data       = base64encode(jsonencode({ period_type = "year" }))
  }

  retry_config {
    retry_count = 1
  }
}

# Trial expiry checker — runs daily at 08:00 UTC.
# Sends 7-day warning emails and trial-expired notifications.
# Uses a ±12h window so each user is caught at most once per pass.
# The 48h look-back on the expired pass survives a single missed run.
resource "google_cloud_scheduler_job" "trial_check" {
  name        = "trial-expiry-check"
  description = "Daily sweep for trials expiring in ~7 days and trials that just expired"
  project     = var.project_id
  region      = var.region
  schedule    = "0 8 * * *"
  time_zone   = "UTC"

  pubsub_target {
    topic_name = google_pubsub_topic.trial_check_trigger.id
    data       = base64encode("{}")
  }

  retry_config {
    retry_count = 1
  }
}

resource "google_cloud_scheduler_job" "parkrun_results_check" {
  name        = "parkrun-results-check"
  description = "Polls parkrun for auto-resolution of waiting pending inputs"
  project     = var.project_id
  region      = var.region
  schedule    = "0 10,12,14,16,18,20 * * *"
  time_zone   = "Europe/London"

  pubsub_target {
    topic_name = google_pubsub_topic.parkrun_results_trigger.id
    data       = base64encode("{}")
  }

  retry_config {
    retry_count = 1
  }
}
