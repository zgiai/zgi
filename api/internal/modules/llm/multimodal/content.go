package multimodal

import (
	"strings"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

const (
	ContentTypeText     = "text"
	ContentTypeImageURL = "image_url"

	ImageDetailHigh = "high"
	ImageDetailAuto = "auto"
)

func IsImageFile(extension string, mimeType string) bool {
	extension = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}
	switch extension {
	case "jpg", "jpeg", "png", "webp", "gif", "svg":
		return true
	default:
		return false
	}
}

func BuildTextPart(text string) adapter.MessageContentPart {
	return adapter.MessageContentPart{
		Type: ContentTypeText,
		Text: text,
	}
}

func BuildImageURLPart(url string, detail string) adapter.MessageContentPart {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = ImageDetailAuto
	}
	return adapter.MessageContentPart{
		Type: ContentTypeImageURL,
		ImageURL: &adapter.ImageURL{
			URL:    strings.TrimSpace(url),
			Detail: detail,
		},
	}
}

func BuildImageDataPart(base64Data string, mimeType string, detail string) adapter.MessageContentPart {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return BuildImageURLPart("data:"+mimeType+";base64,"+base64Data, detail)
}

func BuildUserContent(text string, imageParts []adapter.MessageContentPart) interface{} {
	parts := make([]adapter.MessageContentPart, 0, len(imageParts)+1)
	for _, part := range imageParts {
		if part.Type == ContentTypeImageURL && part.ImageURL != nil && strings.TrimSpace(part.ImageURL.URL) != "" {
			parts = append(parts, part)
		}
	}
	if strings.TrimSpace(text) != "" {
		parts = append(parts, BuildTextPart(text))
	}
	if len(parts) == 0 {
		return strings.TrimSpace(text)
	}
	if len(parts) == 1 && parts[0].Type == ContentTypeText {
		return parts[0].Text
	}
	return parts
}
