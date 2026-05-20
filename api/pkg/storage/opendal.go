package storage

// todo : fix as real openDAL storage
import (
	"fmt"
	"os"
	"path/filepath"

	appconfig "github.com/zgiai/ginext/config"
)

type LocalStorage struct {
	basePath string
}

func NewLocalStorage(basePath string) *LocalStorage {
	return &LocalStorage{
		basePath: basePath,
	}
}

func (s *LocalStorage) Save(key string, data []byte) error {
	filePath := filepath.Join(s.basePath, key)

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

func (s *LocalStorage) Load(key string) ([]byte, error) {
	filePath := filepath.Join(s.basePath, key)
	return os.ReadFile(filePath)
}

func (s *LocalStorage) Delete(key string) error {
	filePath := filepath.Join(s.basePath, key)
	return os.Remove(filePath)
}

func (s *LocalStorage) GetURL(key string) string {
	return fmt.Sprintf("/storage/%s", key)
}

func (s *LocalStorage) Exists(key string) (bool, error) {
	filePath := filepath.Join(s.basePath, key)
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type OpenDALStorage struct {
	basePath     string
	localStorage *LocalStorage
}

func NewOpenDALStorage() *OpenDALStorage {
	basePath := appconfig.Current().Storage.OpenDALBasePath
	if basePath == "" {
		basePath = "./storage/opendal"
	}

	localStorage := NewLocalStorage(basePath)

	return &OpenDALStorage{
		basePath:     basePath,
		localStorage: localStorage,
	}
}

func (s *OpenDALStorage) Save(key string, data []byte) error {
	return s.localStorage.Save(key, data)
}

func (s *OpenDALStorage) Load(key string) ([]byte, error) {
	return s.localStorage.Load(key)
}

func (s *OpenDALStorage) LoadStream(key string) (<-chan []byte, error) {
	data, err := s.localStorage.Load(key)
	if err != nil {
		return nil, err
	}

	ch := make(chan []byte, 1)
	go func() {
		defer close(ch)
		ch <- data
	}()

	return ch, nil
}

func (s *OpenDALStorage) Download(key string, targetPath string) error {
	data, err := s.localStorage.Load(key)
	if err != nil {
		return fmt.Errorf("failed to load file: %w", err)
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (s *OpenDALStorage) Delete(key string) error {
	return s.localStorage.Delete(key)
}

func (s *OpenDALStorage) Exists(key string) (bool, error) {
	return s.localStorage.Exists(key)
}

func (s *OpenDALStorage) List(prefix string) ([]FileInfo, error) {
	return s.localStorage.List(prefix)
}

// List returns all files under the given prefix/directory
func (s *LocalStorage) List(prefix string) ([]FileInfo, error) {
	var files []FileInfo
	searchPath := filepath.Join(s.basePath, prefix)

	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // Directory doesn't exist, return empty list
			}
			return err
		}
		if !info.IsDir() {
			// Get relative key by removing base path
			relPath, _ := filepath.Rel(s.basePath, path)
			files = append(files, FileInfo{
				Key:          relPath,
				Size:         info.Size(),
				LastModified: info.ModTime(),
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return files, nil
}
