package gcs

import (
	"cloud.google.com/go/storage"
	"context"
	"io/ioutil"
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

func (g *GCS) Read(path string) []byte {
	ctx := context.Background()
	obj := g.bucket.Object(path)

	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil
	}

	body, err := ioutil.ReadAll(r)
	if err != nil {
		return nil
	}

	return body
}

func (g *GCS) Exists(path string) bool {
	ctx := context.Background()
	obj := g.bucket.Object(path)
	_, err := obj.NewReader(ctx)
	return err == nil
}
