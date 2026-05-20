package storage

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	appconfig "github.com/zgiai/ginext/config"
)

// AliyunOssStorage
type AliyunOssStorage struct {
	client *oss.Bucket
	config AliyunOssConfig
}

// AliyunOssConfig
type AliyunOssConfig struct {
	Endpoint        string
	BucketName      string
	Folder          string
	AccessKeyID     string
	AccessKeySecret string
	AuthVersion     string
	Region          string
}

// newAliyunOSSStorage
func newAliyunOSSStorage() Storage {
	storageCfg := appconfig.Current().Storage.AliyunOSS
	config := AliyunOssConfig{
		Endpoint:        storageCfg.Endpoint,
		BucketName:      storageCfg.BucketName,
		Folder:          storageCfg.Folder,
		AccessKeyID:     storageCfg.AccessKeyID,
		AccessKeySecret: storageCfg.AccessKeySecret,
		AuthVersion:     storageCfg.AuthVersion,
		Region:          storageCfg.Region,
	}

	client, err := oss.New(config.Endpoint, config.AccessKeyID, config.AccessKeySecret)
	if err != nil {
		panic(err)
	}

	bucket, err := client.Bucket(config.BucketName)
	if err != nil {
		panic(err)
	}

	return &AliyunOssStorage{
		client: bucket,
		config: config,
	}
}

func (s *AliyunOssStorage) Save(key string, data []byte) error {
	objectKey := s.wrapperFolderFilename(key)
	return s.client.PutObject(objectKey, bytes.NewReader(data))
}

func (s *AliyunOssStorage) Load(key string) ([]byte, error) {
	objectKey := s.wrapperFolderFilename(key)
	body, err := s.client.GetObject(objectKey)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

func (s *AliyunOssStorage) LoadStream(key string) (<-chan []byte, error) {
	objectKey := s.wrapperFolderFilename(key)

	ch := make(chan []byte, 1)

	body, err := s.client.GetObject(objectKey)
	if err != nil {
		close(ch)
		return nil, err
	}

	go func() {
		defer close(ch)
		defer body.Close()

		buffer := make([]byte, 4096)
		for {
			n, readErr := body.Read(buffer)
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

func (s *AliyunOssStorage) Download(key string, targetPath string) error {
	objectKey := s.wrapperFolderFilename(key)

	dir := filepath.Dir(targetPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	if err := s.client.GetObjectToFile(objectKey, targetPath); err != nil {
		return fmt.Errorf("failed to download file from OSS: %w", err)
	}

	return nil
}

func (s *AliyunOssStorage) Delete(key string) error {
	objectKey := s.wrapperFolderFilename(key)
	return s.client.DeleteObject(objectKey)
}

func (s *AliyunOssStorage) Exists(key string) (bool, error) {
	objectKey := s.wrapperFolderFilename(key)
	_, err := s.client.GetObjectMeta(objectKey)
	if err != nil {
		if se, ok := err.(oss.ServiceError); ok {
			if se.Code == "NoSuchBucket" || se.Code == "NoSuchKey" {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

func (s *AliyunOssStorage) wrapperFolderFilename(filename string) string {
	if s.config.Folder != "" {
		return s.config.Folder + "/" + filename
	}
	return filename
}

func (s *AliyunOssStorage) List(prefix string) ([]FileInfo, error) {
	objectPrefix := s.wrapperFolderFilename(prefix)
	var files []FileInfo

	marker := ""
	for {
		lsRes, err := s.client.ListObjects(oss.Prefix(objectPrefix), oss.Marker(marker), oss.MaxKeys(1000))
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, object := range lsRes.Objects {
			// Remove folder prefix from key
			key := object.Key
			if s.config.Folder != "" && len(key) > len(s.config.Folder)+1 {
				key = key[len(s.config.Folder)+1:]
			}
			files = append(files, FileInfo{
				Key:          key,
				Size:         object.Size,
				LastModified: object.LastModified,
			})
		}

		if !lsRes.IsTruncated {
			break
		}
		marker = lsRes.NextMarker
	}

	return files, nil
}
