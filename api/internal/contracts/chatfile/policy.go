// Package chatfile defines chat attachment admission independently of parsers
// and model capabilities. It does not grant access to files or execution tools.
package chatfile

import (
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	MiB             = int64(1024 * 1024)
	MaxTurnBytes    = 100 * MiB
	DefaultCount    = 5
	MaxCount        = 10
	MaxRequestBytes = MaxTurnBytes + MiB // Multipart envelope, not file content.

	// maxCustomExtensions bounds an application's explicit extension list.
	maxCustomExtensions = 50
)

// File kinds an application may allow. Custom lists extensions explicitly
// and stands alone.
const (
	KindDocument = "document"
	KindImage    = "image"
	KindAudio    = "audio"
	KindVideo    = "video"
	KindCustom   = "custom"
)

var (
	ErrUnsupportedType = errors.New("unsupported chat attachment type")
	ErrFileTooLarge    = errors.New("chat attachment exceeds file size limit")
	ErrTurnTooLarge    = errors.New("chat attachments exceed per-turn size limit")
	ErrTooManyFiles    = errors.New("too many chat attachments")
)

var extensionPattern = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

// Policy is application-owned and is frozen with the application's version.
// Nil means documents and images, five per turn; an explicit empty list
// permits nothing. The newer fields are omitted when unset so policies saved
// before them keep their exact JSON.
type Policy struct {
	AllowedFileTypes []string `json:"allowed_file_types"`
	// AllowedFileExtensions lists what the custom kind admits, without dots.
	AllowedFileExtensions []string `json:"allowed_file_extensions,omitempty"`
	// NumberLimits is how many files one turn may carry; zero means the default.
	NumberLimits int `json:"number_limits,omitempty"`
}

func (p *Policy) Validate() error {
	if p == nil {
		return nil
	}
	seen := make(map[string]bool, len(p.AllowedFileTypes))
	for _, kind := range p.AllowedFileTypes {
		switch kind {
		case KindDocument, KindImage, KindAudio, KindVideo, KindCustom:
		default:
			return ErrUnsupportedType
		}
		if seen[kind] {
			return ErrUnsupportedType
		}
		seen[kind] = true
	}
	if seen[KindCustom] {
		// Custom names its own extensions and replaces the other kinds.
		if len(p.AllowedFileTypes) != 1 || len(p.AllowedFileExtensions) == 0 || len(p.AllowedFileExtensions) > maxCustomExtensions {
			return ErrUnsupportedType
		}
		unique := make(map[string]bool, len(p.AllowedFileExtensions))
		for _, extension := range p.AllowedFileExtensions {
			if !extensionPattern.MatchString(extension) || slices.Contains(blockedExtensions, extension) || unique[extension] {
				return ErrUnsupportedType
			}
			unique[extension] = true
		}
	} else if len(p.AllowedFileExtensions) > 0 {
		return ErrUnsupportedType
	}
	if p.NumberLimits < 0 || p.NumberLimits > MaxCount {
		return ErrTooManyFiles
	}
	return nil
}

// Normalize lowercases custom extensions and drops leading dots and
// duplicates, so a saved policy compares and validates predictably.
func (p *Policy) Normalize() {
	if p == nil || len(p.AllowedFileExtensions) == 0 {
		return
	}
	normalized := make([]string, 0, len(p.AllowedFileExtensions))
	for _, extension := range p.AllowedFileExtensions {
		extension = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
		if extension != "" && !slices.Contains(normalized, extension) {
			normalized = append(normalized, extension)
		}
	}
	p.AllowedFileExtensions = normalized
}

func Clone(p *Policy) *Policy {
	if p == nil {
		return nil
	}
	return &Policy{AllowedFileTypes: slices.Clone(p.AllowedFileTypes), AllowedFileExtensions: slices.Clone(p.AllowedFileExtensions), NumberLimits: p.NumberLimits}
}

// Effective is a server-produced admission contract. All sizes are bytes.
type Effective struct {
	AllowedFileTypes  []string `json:"allowed_file_types"`
	AllowedExtensions []string `json:"allowed_extensions"`
	NumberLimit       int      `json:"number_limit"`
	DocumentMaxBytes  int64    `json:"document_max_bytes"`
	ImageMaxBytes     int64    `json:"image_max_bytes"`
	AudioMaxBytes     int64    `json:"audio_max_bytes,omitempty"`
	VideoMaxBytes     int64    `json:"video_max_bytes,omitempty"`
	TurnMaxBytes      int64    `json:"turn_max_bytes"`
	// ParsedExtensions are read for the model as text, VisionExtensions are
	// shown to vision models. Other admitted files are kept as files: the
	// model sees their names, and code execution can open them.
	ParsedExtensions []string `json:"parsed_extensions,omitempty"`
	VisionExtensions []string `json:"vision_extensions,omitempty"`
}

// Limits are the platform's per-kind size caps in MiB and its default count.
type Limits struct {
	DocumentMiB int64
	ImageMiB    int64
	AudioMiB    int64
	VideoMiB    int64
	Count       int
}

func Resolve(p *Policy, documentMiB, imageMiB int64, count int) Effective {
	return ResolveLimits(p, Limits{DocumentMiB: documentMiB, ImageMiB: imageMiB, Count: count})
}

// ResolveLimits resolves an application's policy against platform limits.
func ResolveLimits(p *Policy, limits Limits) Effective {
	if limits.DocumentMiB <= 0 {
		limits.DocumentMiB = 50
	}
	if limits.ImageMiB <= 0 {
		limits.ImageMiB = 10
	}
	if limits.AudioMiB <= 0 {
		limits.AudioMiB = 50
	}
	if limits.VideoMiB <= 0 {
		limits.VideoMiB = 100
	}
	count := limits.Count
	if count <= 0 {
		count = DefaultCount
	}
	types := []string{KindDocument, KindImage}
	if p != nil {
		types = slices.Clone(p.AllowedFileTypes)
		if p.NumberLimits > 0 {
			count = min(p.NumberLimits, MaxCount)
		}
	}
	capMiB := MaxTurnBytes / MiB
	result := Effective{AllowedFileTypes: types, NumberLimit: count,
		DocumentMaxBytes: min(limits.DocumentMiB, capMiB) * MiB, ImageMaxBytes: min(limits.ImageMiB, capMiB) * MiB,
		AudioMaxBytes: min(limits.AudioMiB, capMiB) * MiB, VideoMaxBytes: min(limits.VideoMiB, capMiB) * MiB,
		TurnMaxBytes: MaxTurnBytes, ParsedExtensions: DocumentExtensions(), VisionExtensions: VisionExtensions()}
	for _, kind := range types {
		switch kind {
		case KindDocument:
			result.AllowedExtensions = append(result.AllowedExtensions, DocumentExtensions()...)
		case KindImage:
			result.AllowedExtensions = append(result.AllowedExtensions, ImageExtensions()...)
		case KindAudio:
			result.AllowedExtensions = append(result.AllowedExtensions, AudioExtensions()...)
		case KindVideo:
			result.AllowedExtensions = append(result.AllowedExtensions, VideoExtensions()...)
		case KindCustom:
			if p != nil {
				for _, extension := range p.AllowedFileExtensions {
					if !slices.Contains(blockedExtensions, extension) {
						result.AllowedExtensions = append(result.AllowedExtensions, extension)
					}
				}
			}
		}
	}
	return result
}

// DocumentExtensions preserves existing chat document/text formats. This list
// is not the parser capability list and never includes arbitrary executables.
func DocumentExtensions() []string {
	return []string{"txt", "md", "markdown", "mdx", "pdf", "html", "htm", "doc", "docx", "xls", "xlsx", "csv", "eml", "msg", "xml", "epub", "ppt", "pptx", "json", "key", "numbers", "java", "sql"}
}

func ImageExtensions() []string {
	return []string{"jpg", "jpeg", "png", "gif", "webp", "svg"}
}

// VisionExtensions are the images a vision model can see.
func VisionExtensions() []string {
	return []string{"jpg", "jpeg", "png", "gif", "webp"}
}

func AudioExtensions() []string {
	return []string{"mp3", "m4a", "wav", "amr", "mpga"}
}

func VideoExtensions() []string {
	return []string{"mp4", "mov", "webm", "mpeg"}
}

// blockedExtensions are never admitted, even when an application lists them.
var blockedExtensions = []string{"exe", "bat", "cmd", "com", "msi", "dll", "scr", "ps1", "vbs", "jar", "apk", "app", "dmg", "sh"}

// FileKind classifies an extension for size limits and processing.
func FileKind(extension string) string {
	extension = strings.TrimPrefix(strings.ToLower(extension), ".")
	switch {
	case slices.Contains(ImageExtensions(), extension):
		return KindImage
	case slices.Contains(AudioExtensions(), extension):
		return KindAudio
	case slices.Contains(VideoExtensions(), extension):
		return KindVideo
	default:
		return KindDocument
	}
}

// Parsed reports whether the model reads a file of this extension as text.
func Parsed(extension string) bool {
	return slices.Contains(DocumentExtensions(), strings.TrimPrefix(strings.ToLower(extension), "."))
}

func Extension(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
}

func (p Effective) ValidateFile(extension string, size int64) error {
	extension = strings.TrimPrefix(strings.ToLower(extension), ".")
	if !slices.Contains(p.AllowedExtensions, extension) || slices.Contains(blockedExtensions, extension) {
		return ErrUnsupportedType
	}
	limit := p.DocumentMaxBytes
	switch FileKind(extension) {
	case KindImage:
		limit = p.ImageMaxBytes
	case KindAudio:
		if p.AudioMaxBytes > 0 {
			limit = p.AudioMaxBytes
		}
	case KindVideo:
		if p.VideoMaxBytes > 0 {
			limit = p.VideoMaxBytes
		}
	}
	if size < 0 || size > limit {
		return ErrFileTooLarge
	}
	return nil
}

// ValidateTurn uses unique, authorized file references supplied by its caller.
func (p Effective) ValidateTurn(sizes []int64) error {
	if len(sizes) > p.NumberLimit {
		return ErrTooManyFiles
	}
	var total int64
	for _, size := range sizes {
		if size < 0 || size > p.TurnMaxBytes-total {
			return ErrTurnTooLarge
		}
		total += size
	}
	return nil
}

// PlatformEffective admits everything any application may allow, for the
// shared console upload endpoint; each application's own policy is applied
// when the file joins a turn.
func PlatformEffective(limits Limits) Effective {
	effective := ResolveLimits(&Policy{AllowedFileTypes: []string{KindDocument, KindImage, KindAudio, KindVideo}, NumberLimits: MaxCount}, limits)
	return effective
}

// ValidatePlatformFile checks a chat upload on the shared console endpoint:
// any non-blocked extension an application could allow, within its kind's
// size cap. The application's own policy is applied when the file joins a turn.
func ValidatePlatformFile(extension string, size int64, limits Limits) error {
	extension = strings.TrimPrefix(strings.ToLower(extension), ".")
	if !AdmitsAny(extension) {
		return ErrUnsupportedType
	}
	effective := PlatformEffective(limits)
	effective.AllowedExtensions = append(effective.AllowedExtensions, extension)
	return effective.ValidateFile(extension, size)
}

// AdmitsAny reports whether an uploaded file passes the platform's shared
// endpoint: a known kind, or any non-blocked extension applications may list.
func AdmitsAny(extension string) bool {
	extension = strings.TrimPrefix(strings.ToLower(extension), ".")
	return extensionPattern.MatchString(extension) && !slices.Contains(blockedExtensions, extension)
}
