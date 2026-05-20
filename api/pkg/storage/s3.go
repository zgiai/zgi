package storage

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	appconfig "github.com/zgiai/zgi/api/config"
)

// S3Storage AWS S3 storage
type S3Storage struct {
	client   *s3.S3
	uploader *s3manager.Uploader
	config   S3Config
}

// S3Config S3 configuration
type S3Config struct {
	AccessKey        string
	SecretKey        string
	Region           string
	BucketName       string
	Endpoint         string
	S3ForcePathStyle bool
	DisableSSL       bool
	Folder           string
}

// newS3Storage create S3 storage instance
func newS3Storage() Storage {
	storageCfg := appconfig.Current().Storage.S3
	config := S3Config{
		AccessKey:        storageCfg.AccessKey,
		SecretKey:        storageCfg.SecretKey,
		Region:           storageCfg.Region,
		BucketName:       storageCfg.BucketName,
		Endpoint:         storageCfg.Endpoint,
		S3ForcePathStyle: storageCfg.S3ForcePathStyle,
		DisableSSL:       storageCfg.DisableSSL,
		Folder:           storageCfg.Folder,
	}

	// Create AWS session
	creds := credentials.NewStaticCredentials(config.AccessKey, config.SecretKey, "")
	awsConfig := &aws.Config{
		Region:           aws.String(config.Region),
		Credentials:      creds,
		DisableSSL:       aws.Bool(config.DisableSSL),
		S3ForcePathStyle: aws.Bool(config.S3ForcePathStyle),
	}

	// Set custom endpoint if specified
	if config.Endpoint != "" {
		awsConfig.Endpoint = aws.String(config.Endpoint)
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		panic(fmt.Sprintf("failed to create AWS session: %v", err))
	}

	s3Client := s3.New(sess)
	uploader := s3manager.NewUploader(sess)

	return &S3Storage{
		client:   s3Client,
		uploader: uploader,
		config:   config,
	}
}

func (s *S3Storage) Save(key string, data []byte) error {
	objectKey := s.wrapperFolderFilename(key)

	// Upload data using uploader
	_, err := s.uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader(data),
	})

	return err
}

func (s *S3Storage) Load(key string) ([]byte, error) {
	objectKey := s.wrapperFolderFilename(key)

	// Download object
	result, err := s.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()

	return io.ReadAll(result.Body)
}

func (s *S3Storage) LoadStream(key string) (<-chan []byte, error) {
	objectKey := s.wrapperFolderFilename(key)

	// Download object
	result, err := s.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		ch := make(chan []byte)
		close(ch)
		return ch, err
	}

	ch := make(chan []byte, 1)

	go func() {
		defer close(ch)
		defer result.Body.Close()

		buffer := make([]byte, 4096)
		for {
			n, readErr := result.Body.Read(buffer)
			if n > 0 {
				ch <- buffer[:n]
			}
			if readErr != nil {
				if readErr != io.EOF {
					// todo: log error
				}
				break
			}
		}
	}()

	return ch, nil
}

func (s *S3Storage) Download(key string, targetPath string) error {
	objectKey := s.wrapperFolderFilename(key)

	dir := filepath.Dir(targetPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Download file to local path
	file, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", targetPath, err)
	}
	defer file.Close()

	// Download object to file
	result, err := s.client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer result.Body.Close()

	_, err = io.Copy(file, result.Body)
	if err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	return nil

	return nil
}

func (s *S3Storage) Delete(key string) error {
	objectKey := s.wrapperFolderFilename(key)

	// Delete object
	_, err := s.client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(objectKey),
	})

	if err != nil {
		return err
	}

	// Wait for object to be deleted
	if err == nil {
		return s.client.WaitUntilObjectNotExists(&s3.HeadObjectInput{
			Bucket: aws.String(s.config.BucketName),
			Key:    aws.String(objectKey),
		})
	}
	return err
}

func (s *S3Storage) Exists(key string) (bool, error) {
	objectKey := s.wrapperFolderFilename(key)

	// Check if object exists
	_, err := s.client.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(s.config.BucketName),
		Key:    aws.String(objectKey),
	})

	if err != nil {
		// Check if it's a NotFound error
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "NotFound" || aerr.Code() == "NoSuchKey" {
				return false, nil
			}
		}
		return false, err
	}

	return true, nil
}

func (s *S3Storage) wrapperFolderFilename(filename string) string {
	if s.config.Folder != "" {
		return s.config.Folder + "/" + filename
	}
	return filename
}

func (s *S3Storage) List(prefix string) ([]FileInfo, error) {
	objectPrefix := s.wrapperFolderFilename(prefix)
	var files []FileInfo

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.config.BucketName),
		Prefix: aws.String(objectPrefix),
	}

	err := s.client.ListObjectsV2Pages(input, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, obj := range page.Contents {
			// Remove folder prefix from key
			key := *obj.Key
			if s.config.Folder != "" && len(key) > len(s.config.Folder)+1 {
				key = key[len(s.config.Folder)+1:]
			}
			files = append(files, FileInfo{
				Key:          key,
				Size:         *obj.Size,
				LastModified: *obj.LastModified,
			})
		}
		return true // Continue pagination
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	return files, nil
}
