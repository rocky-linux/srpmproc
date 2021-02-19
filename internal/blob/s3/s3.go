package s3

import (
	"bytes"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"io/ioutil"
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

func (s *S3) Read(path string) []byte {
	obj, err := s.uploader.S3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil
	}

	body, err := ioutil.ReadAll(obj.Body)
	if err != nil {
		return nil
	}

	return body
}

func (s *S3) Exists(path string) bool {
	_, err := s.uploader.S3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(path),
	})
	return err == nil
}
