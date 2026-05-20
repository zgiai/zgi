package storage

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
	appconfig "github.com/zgiai/ginext/config"
)

// QiniuStorage qiniu cloud storage
type QiniuStorage struct {
	client *storage.BucketManager
	config QiniuConfig
	mac    *auth.Credentials
}

// QiniuConfig qiniu cloud configuration
type QiniuConfig struct {
	AccessKey string
	SecretKey string
	Bucket    string
	Domain    string
	Zone      string
	UseHTTPS  bool
	Folder    string
}

// newQiniuStorage create a qiniu cloud storage instance
func newQiniuStorage() Storage {
	storageCfg := appconfig.Current().Storage.Qiniu
	config := QiniuConfig{
		AccessKey: storageCfg.AccessKey,
		SecretKey: storageCfg.SecretKey,
		Bucket:    storageCfg.Bucket,
		Domain:    storageCfg.Domain,
		Zone:      storageCfg.Zone,
		UseHTTPS:  storageCfg.UseHTTPS,
		Folder:    storageCfg.Folder,
	}

	mac := auth.New(config.AccessKey, config.SecretKey)
	cfg := storage.Config{}
	// Set storage region
	switch config.Zone {
	case "huadong":
		cfg.Zone = &storage.ZoneHuadong
	case "huabei":
		cfg.Zone = &storage.ZoneHuabei
	case "huanan":
		cfg.Zone = &storage.ZoneHuanan
	case "beimei":
		cfg.Zone = &storage.ZoneBeimei
	default:
		cfg.Zone = &storage.ZoneHuadong
	}

	cfg.UseHTTPS = config.UseHTTPS
	bucketManager := storage.NewBucketManager(mac, &cfg)

	return &QiniuStorage{
		client: bucketManager,
		config: config,
		mac:    mac,
	}
}

func (s *QiniuStorage) Save(key string, data []byte) error {
	objectKey := s.wrapperFolderFilename(key)

	// Build upload policy
	putPolicy := storage.PutPolicy{
		Scope: s.config.Bucket + ":" + objectKey,
	}
	upToken := putPolicy.UploadToken(s.mac)

	// Configure upload parameters
	cfg := storage.Config{}
	switch s.config.Zone {
	case "huadong":
		cfg.Zone = &storage.ZoneHuadong
	case "huabei":
		cfg.Zone = &storage.ZoneHuabei
	case "huanan":
		cfg.Zone = &storage.ZoneHuanan
	case "beimei":
		cfg.Zone = &storage.ZoneBeimei
	default:
		cfg.Zone = &storage.ZoneHuadong
	}
	cfg.UseHTTPS = s.config.UseHTTPS

	// Create form uploader
	formUploader := storage.NewFormUploader(&cfg)

	// Upload data
	ret := storage.PutRet{}
	dataLen := int64(len(data))
	err := formUploader.Put(nil, &ret, upToken, objectKey, bytes.NewReader(data), dataLen, nil)
	return err
}

func (s *QiniuStorage) Load(key string) ([]byte, error) {
	objectKey := s.wrapperFolderFilename(key)

	// Build download URL
	downloadURL := storage.MakePrivateURL(s.mac, s.config.Domain, objectKey, 3600)

	// Download file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func (s *QiniuStorage) LoadStream(key string) (<-chan []byte, error) {
	objectKey := s.wrapperFolderFilename(key)

	ch := make(chan []byte, 1)

	// Build download URL
	downloadURL := storage.MakePrivateURL(s.mac, s.config.Domain, objectKey, 3600)

	// Download file
	resp, err := http.Get(downloadURL)
	if err != nil {
		close(ch)
		return nil, err
	}

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		buffer := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buffer)
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

func (s *QiniuStorage) Download(key string, targetPath string) error {
	objectKey := s.wrapperFolderFilename(key)

	dir := filepath.Dir(targetPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Build download URL
	publicURL := storage.MakePublicURL(s.config.Domain, objectKey)

	// Use HTTP GET request to download file
	resp, err := http.Get(publicURL)
	if err != nil {
		return fmt.Errorf("failed to download file from Qiniu: %w", err)
	}
	defer resp.Body.Close()

	// Create local file
	file, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	// Write downloaded content to local file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write to local file: %w", err)
	}

	return nil
}

func (s *QiniuStorage) Delete(key string) error {
	objectKey := s.wrapperFolderFilename(key)
	return s.client.Delete(s.config.Bucket, objectKey)
}

func (s *QiniuStorage) Exists(key string) (bool, error) {
	objectKey := s.wrapperFolderFilename(key)

	_, err := s.client.Stat(s.config.Bucket, objectKey)
	if err != nil {
		if v, ok := err.(*storage.ErrorInfo); ok {
			if v.Code == 612 { // File not found error code
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

func (s *QiniuStorage) wrapperFolderFilename(filename string) string {
	// Qiniu cloud storage does not use folder concept, but uses object key prefix to simulate folder structure
	// Here we use the path prefix from environment variables
	if s.config.Folder != "" {
		return s.config.Folder + "/" + filename
	}
	return filename
}

func (s *QiniuStorage) List(prefix string) ([]FileInfo, error) {
	objectPrefix := s.wrapperFolderFilename(prefix)
	var files []FileInfo

	marker := ""
	for {
		entries, _, nextMarker, hasNext, err := s.client.ListFiles(s.config.Bucket, objectPrefix, "", marker, 1000)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, entry := range entries {
			// Remove folder prefix from key
			key := entry.Key
			if s.config.Folder != "" && len(key) > len(s.config.Folder)+1 {
				key = key[len(s.config.Folder)+1:]
			}
			files = append(files, FileInfo{
				Key:          key,
				Size:         entry.Fsize,
				LastModified: time.Unix(0, entry.PutTime*100), // Qiniu PutTime is in 100 nanoseconds
			})
		}

		if !hasNext {
			break
		}
		marker = nextMarker
	}

	return files, nil
}
