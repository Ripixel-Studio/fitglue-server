package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// StorageAdapter provides blob storage operations using Google Cloud Storage
type StorageAdapter struct {
	Client *storage.Client
}

func parseURI(bucketName, objectName string) (string, string) {
	if bucketName == "" && strings.HasPrefix(objectName, "gs://") {
		withoutProtocol := strings.TrimPrefix(objectName, "gs://")
		parts := strings.SplitN(withoutProtocol, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}
	return bucketName, objectName
}

func (a *StorageAdapter) Write(ctx context.Context, bucketName, objectName string, data []byte) error {
	bucketName, objectName = parseURI(bucketName, objectName)
	wc := a.Client.Bucket(bucketName).Object(objectName).NewWriter(ctx)
	if _, err := wc.Write(data); err != nil {
		return err
	}
	return wc.Close()
}

func (a *StorageAdapter) Get(ctx context.Context, bucketName, objectName string) ([]byte, error) {
	bucketName, objectName = parseURI(bucketName, objectName)
	rc, err := a.Client.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (a *StorageAdapter) Delete(ctx context.Context, bucketName, objectName string) error {
	bucketName, objectName = parseURI(bucketName, objectName)
	return a.Client.Bucket(bucketName).Object(objectName).Delete(ctx)
}

// ListByPrefix returns the names of all objects under prefix in the bucket.
func (a *StorageAdapter) ListByPrefix(ctx context.Context, bucketName, prefix string) ([]string, error) {
	bucketName, prefix = parseURI(bucketName, prefix)
	it := a.Client.Bucket(bucketName).Objects(ctx, &storage.Query{Prefix: prefix})
	var names []string
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			return names, nil
		}
		if err != nil {
			return nil, err
		}
		names = append(names, attrs.Name)
	}
}

// DeleteByPrefix deletes every object under prefix in the bucket, returning
// the number deleted. Objects that vanish mid-iteration are not an error.
func (a *StorageAdapter) DeleteByPrefix(ctx context.Context, bucketName, prefix string) (int, error) {
	bucketName, prefix = parseURI(bucketName, prefix)
	names, err := a.ListByPrefix(ctx, bucketName, prefix)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, name := range names {
		if err := a.Client.Bucket(bucketName).Object(name).Delete(ctx); err != nil {
			if err == storage.ErrObjectNotExist {
				continue
			}
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// SignedURL generates a V4 signed URL for uploading or downloading an object.
// On Cloud Run with a service account, credentials are auto-detected.
// contentLength is used for PUT uploads; pass 0 for GET downloads.
func (a *StorageAdapter) SignedURL(ctx context.Context, bucketName, objectName, contentType string, contentLength int64, expiry time.Duration) (string, error) {
	bucketName, objectName = parseURI(bucketName, objectName)

	method := "GET"
	var headers []string
	if contentType != "" {
		method = "PUT"
		headers = []string{"Content-Type:" + contentType}
		if contentLength > 0 {
			headers = append(headers, fmt.Sprintf("x-goog-content-length-range:0,%d", contentLength))
		}
	}

	url, err := a.Client.Bucket(bucketName).SignedURL(objectName, &storage.SignedURLOptions{
		Scheme:  storage.SigningSchemeV4,
		Method:  method,
		Expires: time.Now().Add(expiry),
		Headers: headers,
	})
	if err != nil {
		return "", err
	}
	return url, nil
}
