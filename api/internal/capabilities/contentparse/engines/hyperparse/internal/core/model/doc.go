package model

// Document is the unified internal representation for all supported formats.
type Document struct {
	ID        string
	Format    string
	Title     string
	PageCount int
	Metadata  map[string]string
	Sections  []Section
}

type Section struct {
	Path    string
	Heading string
	Blocks  []Block
}

type BBox struct {
	Left   float64
	Top    float64
	Right  float64
	Bottom float64
}

type Block struct {
	Type      string
	Text      string
	Page      int
	Order     int
	TraceID   string
	BBox      *BBox
	Precision string
	Payload   map[string]any
}
