resource "google_firestore_database" "database" {
  project     = var.project_id
  name        = "(default)"
  location_id = var.region
  type        = "FIRESTORE_NATIVE"

  depends_on = [google_project_service.apis]
}

resource "google_firestore_index" "executions_service_timestamp" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "executions"

  fields {
    field_path = "service"
    order      = "ASCENDING"
  }

  fields {
    field_path = "timestamp"
    order      = "DESCENDING"
  }
}

resource "google_firestore_index" "executions_status_timestamp" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "executions"

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "timestamp"
    order      = "DESCENDING"
  }
}

resource "google_firestore_index" "executions_user_timestamp" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "executions"

  fields {
    field_path = "user_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "timestamp"
    order      = "DESCENDING"
  }
}

resource "google_firestore_field" "executions_expire_at" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "executions"
  field      = "expire_at"

  ttl_config {}
}

resource "google_firestore_field" "showcased_activities_expires_at" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "showcased_activities"
  field      = "expires_at"

  ttl_config {}
}

# Showcase view de-duplication markers self-expire (~48h). The daily-salted
# visitor hash already rotates daily; TTL just reclaims the storage.
resource "google_firestore_field" "showcase_view_dedup_expires_at" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "showcase_view_dedup"
  field      = "expires_at"

  ttl_config {}
}

resource "google_firestore_index" "pending_inputs_user_status_created" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "pending_inputs"

  fields {
    field_path = "user_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}


resource "google_firestore_index" "executions_pipeline_timestamp" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "executions"

  fields {
    field_path = "pipeline_execution_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "timestamp"
    order      = "DESCENDING"
  }
}

resource "google_firestore_index" "executions_pipeline_timestamp_asc" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "executions"

  fields {
    field_path = "pipeline_execution_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "timestamp"
    order      = "ASCENDING"
  }
}

# Loop Prevention Indexes - check if external ID exists as destination
# Note: These are collection group indexes on subcollection 'activities'
# under users/{userId}/activities


# Phase 2 Performance Optimization Indexes
# Added for O(1) activity list queries

# Index for querying unsynchronized activities (executions without a sync)
resource "google_firestore_index" "executions_user_sync_timestamp" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "executions"

  fields {
    field_path = "user_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "has_synchronized_activity"
    order      = "ASCENDING"
  }

  fields {
    field_path = "timestamp"
    order      = "DESCENDING"
  }
}

# Note: `google_firestore_index` (this resource type) only accepts composite
# (2+ field) indexes — the previous `activities_synced_at` and
# `activities_pipeline_execution` single-field attempts were rejected by GCP
# with "this index is not necessary" for that reason, NOT because collection-group
# single-field indexes are unneeded in general. Automatic single-field indexes
# default to COLLECTION scope only; a collection-group single-field query (e.g.
# CollectionGroup("activities").OrderBy("synced_at", ...) with no other filter)
# still needs an explicit `google_firestore_field` override with a
# COLLECTION_GROUP index_config entry. See pipeline_runs_created_at below for
# a worked example — its absence caused a prod outage in AdminListPipelineRuns.
#
# If you need COMPOSITE collection group indexes (2+ fields), add them here.
# Example:
# resource "google_firestore_index" "activities_user_synced" {
#   project          = var.project_id
#   database         = google_firestore_database.database.name
#   collection       = "activities"
#   query_scope      = "COLLECTION_GROUP"
#
#   fields {
#     field_path = "user_id"
#     order      = "ASCENDING"
#   }
#
#   fields {
#     field_path = "synced_at"
#     order      = "DESCENDING"
#   }
# }

# Index for parkrun-results-source to query pending inputs by enricher
# This is a collection group query since pending_inputs may be queried across users
resource "google_firestore_index" "pending_inputs_enricher_status" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pending_inputs"
  query_scope = "COLLECTION_GROUP"

  fields {
    field_path = "enricher_provider_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }
}

# Index for pending_inputs subcollection (users/{userId}/pending_inputs)
# Used by useRealtimeInputs for real-time dashboard queries
resource "google_firestore_index" "pending_inputs_subcollection_status_created" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pending_inputs"
  query_scope = "COLLECTION"

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}

# -------------------------------------------------------------------
# Pipeline Runs Indexes
# -------------------------------------------------------------------

# Index for listing pipeline runs by status + created_at (descending)
# Used by usePipelineRuns for dashboard activity list
resource "google_firestore_index" "pipeline_runs_status_created" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pipeline_runs"
  query_scope = "COLLECTION"

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}

# Index for counting pipeline runs by status + created_at (ascending)
# Used by countSynced for stats queries with >= date filter
resource "google_firestore_index" "pipeline_runs_status_created_asc" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pipeline_runs"
  query_scope = "COLLECTION"

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "ASCENDING"
  }
}

# Index for filtering by source + created_at
# Used for filtering activities by source (e.g., "From Hevy")
resource "google_firestore_index" "pipeline_runs_source_created" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pipeline_runs"
  query_scope = "COLLECTION"

  fields {
    field_path = "source"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}

# Index for finding pipeline runs by activity_id + created_at
# Used by findByActivityId() in repost-handler, inputs-handler (dismiss), etc.
resource "google_firestore_index" "pipeline_runs_activity_created" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pipeline_runs"
  query_scope = "COLLECTION"

  fields {
    field_path = "activity_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}

# Index for getSynced() - activity_id + status + created_at
# Used when looking up synced runs by activity ID
resource "google_firestore_index" "pipeline_runs_activity_status_created" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pipeline_runs"
  query_scope = "COLLECTION"

  fields {
    field_path = "activity_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}

# Index for filtering by pipeline_id + created_at
# Used for viewing runs from a specific pipeline config
resource "google_firestore_index" "pipeline_runs_pipeline_created" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pipeline_runs"
  query_scope = "COLLECTION"

  fields {
    field_path = "pipeline_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}

# -------------------------------------------------------------------
# Admin Pipeline Runs - Collection Group Indexes
# Used by admin handler to query pipeline_runs across all users
# -------------------------------------------------------------------

# Collection group index for status + created_at
# Used by admin handler when filtering by status
resource "google_firestore_index" "pipeline_runs_cg_status_created" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pipeline_runs"
  query_scope = "COLLECTION_GROUP"

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}

# Single-field index override for created_at at COLLECTION_GROUP scope.
# The unfiltered admin listing (no status/source) runs a plain
# CollectionGroup("pipeline_runs").OrderBy("created_at", Desc) query. Firestore's
# automatic single-field indexes default to COLLECTION scope only — they do NOT
# cover collection-group queries — so that query fails with FailedPrecondition
# without this override. (The earlier note here claiming this wasn't needed was
# wrong; the composite indexes above only cover the status/source-filtered cases.)
# Ascending/descending COLLECTION entries are repeated to preserve the automatic
# defaults, since specifying any index for a field replaces the full auto set.
resource "google_firestore_field" "pipeline_runs_created_at" {
  project    = var.project_id
  database   = google_firestore_database.database.name
  collection = "pipeline_runs"
  field      = "created_at"

  index_config {
    indexes {
      order       = "ASCENDING"
      query_scope = "COLLECTION"
    }
    indexes {
      order       = "DESCENDING"
      query_scope = "COLLECTION"
    }
    indexes {
      order       = "DESCENDING"
      query_scope = "COLLECTION_GROUP"
    }
  }
}

# -------------------------------------------------------------------
# Uploaded Activities Indexes (Loop Prevention)
# Used by bounceback detection in webhook-processor (TypeScript)
# and GetUploadedActivity (Go) to check if an activity was uploaded by us
# -------------------------------------------------------------------

# Composite index for destination + destination_id queries
# Query: .where('destination', '==', ...).where('destination_id', '==', ...)
resource "google_firestore_index" "uploaded_activities_destination_id" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "uploaded_activities"
  query_scope = "COLLECTION"

  fields {
    field_path = "destination"
    order      = "ASCENDING"
  }

  fields {
    field_path = "destination_id"
    order      = "ASCENDING"
  }
}

# -------------------------------------------------------------------
# Showcased Roundups Indexes
# Used by ListRecentRoundups: .Where("slug","==",slug).OrderBy("period_end", Desc)
# Ordered by period_end (last date covered) so the roundup covering the most
# recent dates surfaces first, regardless of period type (week/month/year).
# -------------------------------------------------------------------

resource "google_firestore_index" "showcased_roundups_slug_period_end" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "showcased_roundups"
  query_scope = "COLLECTION"

  fields {
    field_path = "slug"
    order      = "ASCENDING"
  }

  fields {
    field_path = "period_end"
    order      = "DESCENDING"
  }
}

# -------------------------------------------------------------------
# Showcased Activities Indexes
# Used by ShowcaseStore.listByUserId in showcase-management-handler
# -------------------------------------------------------------------

# Composite index for user_id + created_at (descending)
# Query: .where('user_id', '==', userId).orderBy('created_at', 'desc')
resource "google_firestore_index" "showcased_activities_user_created" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "showcased_activities"
  query_scope = "COLLECTION"

  fields {
    field_path = "user_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}

# Collection group index for source + created_at
# Used by admin handler when filtering by source
resource "google_firestore_index" "pipeline_runs_cg_source_created" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "pipeline_runs"
  query_scope = "COLLECTION_GROUP"

  fields {
    field_path = "source"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "DESCENDING"
  }
}

# -------------------------------------------------------------------
# Admin Audit Log Index
# Used by the admin console to list a single user's audit trail:
#   .where('target_user_id','==',uid).orderBy('timestamp','desc')
# The unfiltered, timestamp-ordered listing uses the automatic single-field index.
# -------------------------------------------------------------------
resource "google_firestore_index" "admin_audit_log_target_timestamp" {
  project     = var.project_id
  database    = google_firestore_database.database.name
  collection  = "admin_audit_log"
  query_scope = "COLLECTION"

  fields {
    field_path = "target_user_id"
    order      = "ASCENDING"
  }

  fields {
    field_path = "timestamp"
    order      = "DESCENDING"
  }
}
