package chunking

type BBox struct {
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
}

type Chunk struct {
	ChunkID     string         `json:"chunk_id"`
	Type        string         `json:"type"`
	Text        string         `json:"text,omitempty"`
	PageIndex   int            `json:"page_index"`
	Order       int            `json:"order"`
	Source      string         `json:"source"`
	Confidence  float64        `json:"confidence"`
	SourceTrace string         `json:"source_trace,omitempty"`
	BBox        *BBox          `json:"bbox,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

type PageGeom struct {
	Left   float64
	Bottom float64
	Right  float64
	Top    float64
}

type PageRef struct {
	PageIndex    int
	ObjectNumber int
}

type TextLike struct {
	Order       int
	SourceTrace string
	Text        string
	ChunkType   string
	GeomX       float64
	GeomY       float64
	BBox        *BBox
	// SegKeyBase is the original PDF segment order before expansion, used for
	// chunk_id=seg_<SegKeyBase>. Split items use ChunkKey.
	SegKeyBase int
	// ChunkKey is used directly as chunk_id when present, for example seg_12_m0.
	ChunkKey string
}

type GeometryLineLike struct {
	Order       int
	SourceTrace string
	Text        string
	PageIndex   int
	GeomX       float64
	GeomY       float64
	BBox        *BBox
}

type GeometryTokenLike struct {
	Order       int
	SourceTrace string
	Text        string
	PageIndex   int
	GeomX       float64
	GeomY       float64
	BBox        *BBox
	FontKey     string
	BaseFont    string
	FontSizePt  float64
}

type ImageLike struct {
	PageIndex     int
	PageObject    int
	XObjectName   string
	ObjectNumber  int
	Format        string
	Width         int
	Height        int
	ByteSize      int
	DecodeWarning string
}

type BookmarkLike struct {
	Title      string
	PageObject int
	Object     int
	Level      int
	Dest       string
	TargetRaw  string
	TargetKind string
}

type AnnotationLike struct {
	PageIndex    int
	ObjectNumber int
	Subtype      string
	Rect         string
	Contents     string
}

type FormLike struct {
	ObjectNumber int
	Name         string
	AltName      string
	FieldType    string
	Value        string
	Flags        int
	PageObject   int
	Rect         string
}

type AttachmentLike struct {
	FileSpecObject    int
	FileName          string
	UnicodeFileName   string
	EmbeddedFileObj   int
	EmbeddedSizeBytes int
	EmbeddedSubtype   string
}

type BuildInput struct {
	Source         string
	PageGeoms      map[int]PageGeom
	Pages          []PageRef
	Texts          []TextLike
	GeometryLines  []GeometryLineLike
	GeometryTokens []GeometryTokenLike
	Images         []ImageLike
	Bookmarks      []BookmarkLike
	Annotations    []AnnotationLike
	Forms          []FormLike
	Attachments    []AttachmentLike
	// SkipTableRule disables table/table_debug rules for performance isolation
	// or temporary table-block shutdowns.
	SkipTableRule bool
}
