package image

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/disintegration/imaging"
)

const (
	IconMaxWidth  = 200
	IconMaxHeight = 200

	PreviewMaxWidth    = 1600
	PreviewMaxHeight   = 1600
	PreviewJPEGQuality = 80
)

func ResizeIconIfNeeded(file multipart.File, header *multipart.FileHeader) ([]byte, error) {
	img, format, err := image.Decode(file)
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	if origWidth <= IconMaxWidth && origHeight <= IconMaxHeight {
		return readAllAndSeek(file, header)
	}

	resized := imaging.Thumbnail(img, IconMaxWidth, IconMaxHeight, imaging.Lanczos)

	var buf bytes.Buffer
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 85})
	case "png":
		err = png.Encode(&buf, resized)
	case "gif":
		err = gif.Encode(&buf, resized, nil)
	default:
		err = png.Encode(&buf, resized)
	}

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func ProcessIconImage(imageData []byte) ([]byte, error) {
	if len(imageData) == 0 {
		return nil, nil
	}

	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return imageData, nil
	}

	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	if origWidth <= IconMaxWidth && origHeight <= IconMaxHeight {
		return imageData, nil
	}

	resized := imaging.Thumbnail(img, IconMaxWidth, IconMaxHeight, imaging.Lanczos)

	var buf bytes.Buffer
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 85})
	case "png":
		err = png.Encode(&buf, resized)
	case "gif":
		err = gif.Encode(&buf, resized, nil)
	default:
		err = png.Encode(&buf, resized)
	}

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func CompressPreviewImage(imageData []byte) ([]byte, string, error) {
	if len(imageData) == 0 {
		return nil, "", nil
	}

	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, "", err
	}

	bounds := img.Bounds()
	target := img
	if bounds.Dx() > PreviewMaxWidth || bounds.Dy() > PreviewMaxHeight {
		target = imaging.Thumbnail(img, PreviewMaxWidth, PreviewMaxHeight, imaging.Lanczos)
	}

	var buf bytes.Buffer
	switch strings.ToLower(format) {
	case "png":
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, target)
		return buf.Bytes(), "image/png", err
	case "jpeg", "jpg":
		err = jpeg.Encode(&buf, target, &jpeg.Options{Quality: PreviewJPEGQuality})
		return buf.Bytes(), "image/jpeg", err
	default:
		err = jpeg.Encode(&buf, target, &jpeg.Options{Quality: PreviewJPEGQuality})
		return buf.Bytes(), "image/jpeg", err
	}
}

func IsImageFile(header *multipart.FileHeader) bool {
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		ext := strings.ToLower(header.Filename)
		return strings.HasSuffix(ext, ".jpg") ||
			strings.HasSuffix(ext, ".jpeg") ||
			strings.HasSuffix(ext, ".png") ||
			strings.HasSuffix(ext, ".gif") ||
			strings.HasSuffix(ext, ".webp")
	}
	return strings.HasPrefix(contentType, "image/")
}

func DownloadAndProcessImage(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return ProcessIconImage(data)
}

func readAllAndSeek(file multipart.File, header *multipart.FileHeader) ([]byte, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		return nil, err
	}
	_ = header
	return data, nil
}
