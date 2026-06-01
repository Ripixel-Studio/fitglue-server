package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		name           string
		bucketName     string
		objectName     string
		expectedBucket string
		expectedObject string
	}{
		{
			name:           "explicit bucket and object passed through unchanged",
			bucketName:     "my-bucket",
			objectName:     "path/to/file.json",
			expectedBucket: "my-bucket",
			expectedObject: "path/to/file.json",
		},
		{
			name:           "gs:// URI parsed when bucket is empty",
			bucketName:     "",
			objectName:     "gs://my-bucket/path/to/file.json",
			expectedBucket: "my-bucket",
			expectedObject: "path/to/file.json",
		},
		{
			name:           "gs:// URI with nested path",
			bucketName:     "",
			objectName:     "gs://my-bucket/a/b/c/file.fit",
			expectedBucket: "my-bucket",
			expectedObject: "a/b/c/file.fit",
		},
		{
			name:           "explicit bucket takes precedence over gs:// prefix",
			bucketName:     "override-bucket",
			objectName:     "gs://other-bucket/file.json",
			expectedBucket: "override-bucket",
			expectedObject: "gs://other-bucket/file.json",
		},
		{
			name:           "non-gs URI without bucket name passed through",
			bucketName:     "",
			objectName:     "plain-object-name",
			expectedBucket: "",
			expectedObject: "plain-object-name",
		},
		{
			name:           "gs:// URI missing object path",
			bucketName:     "",
			objectName:     "gs://bucket-only",
			expectedBucket: "",
			expectedObject: "gs://bucket-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, obj := parseURI(tt.bucketName, tt.objectName)
			assert.Equal(t, tt.expectedBucket, bucket, "bucket mismatch")
			assert.Equal(t, tt.expectedObject, obj, "object mismatch")
		})
	}
}
