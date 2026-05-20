package storage

import (
	"fmt"
	"time"

	appconfig "github.com/zgiai/ginext/config"
)

// FileInfo represents file metadata returned by List
type FileInfo struct {
	Key          string    // File path/key
	Size         int64     // File size in bytes
	LastModified time.Time // Last modification time
}

type StorageType string

const (
	StorageTypeAliyunOSS     StorageType = "aliyun-oss"
	StorageTypeAzureBlob     StorageType = "azure-blob"
	StorageTypeBaiduOBS      StorageType = "baidu-obs"
	StorageTypeGoogleStorage StorageType = "google-storage"
	StorageTypeHuaweiOBS     StorageType = "huawei-obs"
	StorageTypeLocal         StorageType = "local"
	StorageTypeOciStorage    StorageType = "oci-storage"
	StorageTypeOpenDAL       StorageType = "opendal"
	StorageTypeQiniu         StorageType = "qiniu"
	StorageTypeS3            StorageType = "s3"
	StorageTypeTencentCOS    StorageType = "tencent-cos"
	StorageTypeVolcengineCOS StorageType = "volcengine-tos"
	StorageTypeSupabase      StorageType = "supabase"
)

type Storage interface {
	Save(filename string, data []byte) error

	Load(filename string) ([]byte, error)

	LoadStream(filename string) (<-chan []byte, error)

	Download(filename string, targetPath string) error

	Exists(filename string) (bool, error)

	Delete(filename string) error

	// List returns all files under the given prefix/directory
	List(prefix string) ([]FileInfo, error)
}

func GetStorage() Storage {
	storageType := appconfig.Current().Storage.Type

	switch StorageType(storageType) {

	case StorageTypeAliyunOSS:
		return newAliyunOSSStorage()
	case StorageTypeOpenDAL:
		return NewOpenDALStorage()
	case StorageTypeLocal:
		return NewOpenDALStorage()
	case StorageTypeQiniu:
		return newQiniuStorage()
	case StorageTypeS3:
		return newS3Storage()
	// todo: add more storage types
	default:
		panic(fmt.Sprintf("Unsupported storage type: %s", storageType))
	}
}
