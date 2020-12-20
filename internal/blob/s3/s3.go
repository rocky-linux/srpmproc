package s3

import (
	"bytes"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"log"
)

type S3 struct {
	bucket   string
	uploader *s3manager.Uploader
}

func New(name string) *S3 {
	sess := session.Must(session.NewSession())
	uploader := s3manager.NewUploader(sess)

	return &S3{
		bucket:   name,
		uploader: uploader,
	}
}

func (s *S3) Write(path string, content []byte) {
	buf := bytes.NewBuffer(content)

	_, err := s.uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
		Body:   buf,
	})
	if err != nil {
		log.Fatalf("failed to upload file to s3, %v", err)
	}
}
