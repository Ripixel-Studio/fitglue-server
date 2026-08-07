variable "project_id" {
  description = "The ID of the GCP project"
  type        = string
}

variable "region" {
  description = "The GCP region to deploy to"
  type        = string
  default     = "us-central1"
}

variable "log_level" {
  description = "Log level for applications (debug, info, warn, error)"
  type        = string
  default     = "info"
}

variable "environment" {
  description = "The deployment environment (dev, test, prod)"
  type        = string
}

variable "domain_name" {
  description = "Custom domain for Firebase Hosting"
  type        = string
}

variable "base_url" {
  description = "Base URL for the application (used for OAuth redirects)"
  type        = string
}

variable "sentry_org" {
  description = "Sentry organization slug"
  type        = string
  default     = "ripixel-studio"
}


variable "sentry_project" {
  description = "Sentry project slug for server functions"
  type        = string
  default     = "server"
}

variable "sentry_dsn" {
  description = "Sentry DSN for server functions"
  type        = string
  default     = "fitglue-server-dev"
}

variable "image_tag" {
  description = "Docker image tag for Cloud Run services (git SHA in CI, 'latest' for local)"
  type        = string
  default     = "latest"
}
