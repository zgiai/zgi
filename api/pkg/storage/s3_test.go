package storage

import (
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

func TestS3StoragePresignedGetURL(t *testing.T) {
	sess, err := session.NewSession(&aws.Config{
		Region:           aws.String("auto"),
		Credentials:      credentials.NewStaticCredentials("access-key", "secret-key", ""),
		Endpoint:         aws.String("https://storage.example.com"),
		S3ForcePathStyle: aws.Bool(true),
	})
	if err != nil {
		t.Fatalf("session.NewSession() error = %v", err)
	}
	backend := &S3Storage{
		client: s3.New(sess),
		config: S3Config{
			BucketName: "private-bucket",
			Folder:     "tenant-files",
		},
	}

	rawURL, err := backend.PresignedGetURL("music/track.mp3", PresignedGetOptions{
		Expires:             10 * time.Minute,
		ResponseContentType: "audio/mpeg",
	})
	if err != nil {
		t.Fatalf("PresignedGetURL() error = %v", err)
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	if parsedURL.Path != "/private-bucket/tenant-files/music/track.mp3" {
		t.Errorf("path = %q, want object path", parsedURL.Path)
	}
	if got := parsedURL.Query().Get("X-Amz-Expires"); got != "600" {
		t.Errorf("X-Amz-Expires = %q, want %q", got, "600")
	}
	if got := parsedURL.Query().Get("response-content-type"); got != "audio/mpeg" {
		t.Errorf("response-content-type = %q, want %q", got, "audio/mpeg")
	}
	if parsedURL.Query().Get("X-Amz-Signature") == "" {
		t.Error("X-Amz-Signature is empty")
	}
}

func TestS3StoragePresignedGetURLRejectsNonPositiveExpiry(t *testing.T) {
	backend := &S3Storage{}
	if _, err := backend.PresignedGetURL("music/track.mp3", PresignedGetOptions{}); err == nil {
		t.Fatal("PresignedGetURL() error = nil, want validation error")
	}
}
