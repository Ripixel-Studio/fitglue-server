package shared

const (
	ProjectID = "fitglue-project" // Can be overridden by env var in main if needed

	TopicRawActivity           = "topic-raw-activity"
	TopicPipelineActivity      = "topic-pipeline-activity"
	TopicEnrichedActivity      = "topic-enriched-activity"
	TopicDestinationUpload     = "topic-destination-upload"
	TopicJobUploadStrava       = "topic-job-upload-strava"
	TopicFitbitUpdates         = "topic-fitbit-updates"
	TopicEnrichmentLag         = "topic-enrichment-lag"
	TopicParkrunResultsTrigger = "topic-parkrun-results-trigger"
	TopicDataExport            = "topic-data-export"

	CollectionUsers      = "users"
	CollectionCursors    = "cursors"
	CollectionExecutions = "executions"
)
