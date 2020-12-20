package gcs

import (
	"cloud.google.com/go/storage"
	"context"
	"log"
)

type GCS struct {
	bucket *storage.BucketHandle
}

func New(name string) *GCS {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("could not create gcloud client: %v", err)
	}

	return &GCS{
		bucket: client.Bucket(name),
	}
}

func (g *GCS) Write(path string, content []byte) {
	ctx := context.Background()
	obj := g.bucket.Object(path)
	w := obj.NewWriter(ctx)

	_, err := w.Write(content)
	if err != nil {
		log.Fatalf("could not write file to gcs: %v", err)
	}

	// Close, just like writing a file.
	if err := w.Close(); err != nil {
		log.Fatalf("could not close gcs writer to source: %v", err)
	}
}
