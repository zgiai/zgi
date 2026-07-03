package file

import (
	"fmt"
	"net/url"
	"strings"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/storage"
)

func IsWorkflowImageInputFile(extension, mimeType string) bool {
	return InferFileType(extension, mimeType) == FileTypeImage
}

func BuildWorkflowImageInputPublicStorageURL(cfg *appconfig.Config, storageType, uploadFileKey string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("workflow image input public storage URL requires config")
	}

	objectKey, err := workflowImageInputObjectKey(cfg, storageType, uploadFileKey)
	if err != nil {
		return "", err
	}

	if publicBaseURL := strings.TrimSpace(cfg.Workflow.ImageInputPublicBaseURL); publicBaseURL != "" {
		return joinWorkflowImageInputPublicBaseURL(publicBaseURL, objectKey)
	}

	switch storage.StorageType(normalizeWorkflowImageInputStorageType(cfg, storageType)) {
	case storage.StorageTypeAliyunOSS:
		return buildAliyunOSSWorkflowImageInputURL(cfg.Storage.AliyunOSS, objectKey)
	case storage.StorageTypeQiniu:
		return buildQiniuWorkflowImageInputURL(cfg.Storage.Qiniu, objectKey)
	case storage.StorageTypeS3, storage.StorageTypeLocal, storage.StorageTypeOpenDAL:
		return "", fmt.Errorf("%s is required for STORAGE_TYPE=%s when %s=%s",
			"WORKFLOW_IMAGE_INPUT_PUBLIC_BASE_URL",
			normalizeWorkflowImageInputStorageType(cfg, storageType),
			"WORKFLOW_IMAGE_INPUT_URL_MODE",
			appconfig.WorkflowImageInputURLModePublicStorageURL,
		)
	default:
		return "", fmt.Errorf("automatic workflow image input public URL is not supported for STORAGE_TYPE=%s; configure %s",
			normalizeWorkflowImageInputStorageType(cfg, storageType),
			"WORKFLOW_IMAGE_INPUT_PUBLIC_BASE_URL",
		)
	}
}

func workflowImageInputObjectKey(cfg *appconfig.Config, storageType, uploadFileKey string) (string, error) {
	key := strings.TrimLeft(strings.TrimSpace(uploadFileKey), "/")
	if key == "" {
		return "", fmt.Errorf("upload_files.key is required to build workflow image input public URL")
	}

	folder := workflowImageInputStorageFolder(cfg, storageType)
	if folder == "" {
		return key, nil
	}
	return folder + "/" + key, nil
}

func workflowImageInputStorageFolder(cfg *appconfig.Config, storageType string) string {
	if cfg == nil {
		return ""
	}

	switch storage.StorageType(normalizeWorkflowImageInputStorageType(cfg, storageType)) {
	case storage.StorageTypeAliyunOSS:
		return normalizeWorkflowImageInputPathPrefix(cfg.Storage.AliyunOSS.Folder)
	case storage.StorageTypeQiniu:
		return normalizeWorkflowImageInputPathPrefix(cfg.Storage.Qiniu.Folder)
	case storage.StorageTypeS3:
		return normalizeWorkflowImageInputPathPrefix(cfg.Storage.S3.Folder)
	default:
		return ""
	}
}

func normalizeWorkflowImageInputStorageType(cfg *appconfig.Config, storageType string) string {
	normalized := strings.ToLower(strings.TrimSpace(storageType))
	if normalized != "" {
		return normalized
	}
	if cfg == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(cfg.Storage.Type))
}

func normalizeWorkflowImageInputPathPrefix(prefix string) string {
	return strings.Trim(strings.TrimSpace(prefix), "/")
}

func joinWorkflowImageInputPublicBaseURL(baseURL, objectKey string) (string, error) {
	parsed, err := parseWorkflowImageInputHTTPBaseURL(baseURL, "WORKFLOW_IMAGE_INPUT_PUBLIC_BASE_URL")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(parsed.String(), "/") + "/" + strings.TrimLeft(objectKey, "/"), nil
}

func buildAliyunOSSWorkflowImageInputURL(ossCfg appconfig.AliyunOSSStorageConfig, objectKey string) (string, error) {
	bucket := strings.TrimSpace(ossCfg.BucketName)
	if bucket == "" {
		return "", fmt.Errorf("ALIYUN_OSS_BUCKET_NAME is required to build workflow image input public URL")
	}
	endpoint, err := normalizeWorkflowImageInputHost(ossCfg.Endpoint, "ALIYUN_OSS_ENDPOINT")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s.%s/%s", bucket, endpoint, strings.TrimLeft(objectKey, "/")), nil
}

func buildQiniuWorkflowImageInputURL(qiniuCfg appconfig.QiniuStorageConfig, objectKey string) (string, error) {
	domain, err := normalizeWorkflowImageInputHost(qiniuCfg.Domain, "QINIU_DOMAIN")
	if err != nil {
		return "", err
	}

	scheme := "http"
	if qiniuCfg.UseHTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s", scheme, domain, strings.TrimLeft(objectKey, "/")), nil
}

func normalizeWorkflowImageInputHost(rawValue, envName string) (string, error) {
	trimmed := strings.TrimSpace(rawValue)
	if trimmed == "" {
		return "", fmt.Errorf("%s is required to build workflow image input public URL", envName)
	}

	valueForParse := trimmed
	if !strings.Contains(valueForParse, "://") {
		valueForParse = "https://" + valueForParse
	}

	parsed, err := url.Parse(valueForParse)
	if err != nil {
		return "", fmt.Errorf("%s is invalid: %w", envName, err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%s must include a host", envName)
	}
	if strings.Trim(parsed.Path, "/") != "" {
		return "", fmt.Errorf("%s must be a host, not a URL path", envName)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%s must be a host without query or fragment", envName)
	}
	return parsed.Host, nil
}

func parseWorkflowImageInputHTTPBaseURL(rawURL, envName string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("%s is invalid: %w", envName, err)
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return nil, fmt.Errorf("%s must be an absolute http or https URL", envName)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("%s must use http or https", envName)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%s must not include query or fragment", envName)
	}
	return parsed, nil
}
