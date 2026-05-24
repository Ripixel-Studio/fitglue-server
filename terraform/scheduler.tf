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
