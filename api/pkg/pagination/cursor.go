package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CursorPaginator implements cursor-based pagination.
// Better for large datasets and real-time data where offset pagination
// can skip or duplicate items.
//
// Benefits over offset pagination:
//   - Consistent results even when data changes
//   - Better performance on large datasets (no OFFSET)
//   - Works well with infinite scroll UIs
//
// Example:
//
//	paginator, err := pagination.NewCursor[Post](c, db, "created_at", "desc")
type CursorPaginator[T any] struct {
	items      []T
	perPage    int
	cursor     *Cursor
	nextCursor *string
	prevCursor *string
	hasMore    bool
	path       string
}

// Cursor represents the pagination cursor state.
type Cursor struct {
	Field     string `json:"f"`
	Value     any    `json:"v"`
	ID        uint   `json:"i"`
	Direction string `json:"d"` // "next" or "prev"
}

// Encode encodes the cursor to a base64 string.
func (c *Cursor) Encode() string {
	data, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(data)
}

// DecodeCursor decodes a base64 cursor string.
func DecodeCursor(encoded string) (*Cursor, error) {
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor encoding: %w", err)
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("invalid cursor format: %w", err)
	}

	return &cursor, nil
}

// CursorRequest represents cursor pagination parameters.
type CursorRequest struct {
	Cursor   string `form:"cursor" json:"cursor"`
	PageSize int    `form:"page_size" json:"page_size"`
}

// GetPageSize returns items per page with bounds.
func (r *CursorRequest) GetPageSize() int {
	if r.PageSize < 1 {
		return DefaultPageSize
	}
	if r.PageSize > MaxPageSize {
		return MaxPageSize
	}
	return r.PageSize
}

// CursorFromContext extracts cursor request from Gin context.
func CursorFromContext(c *gin.Context) *CursorRequest {
	req := &CursorRequest{}
	req.Cursor = c.Query("cursor")

	if pageSize, err := fmt.Sscanf(c.DefaultQuery("page_size", strconv.Itoa(DefaultPageSize)), "%d", &req.PageSize); err != nil || pageSize < 1 {
		req.PageSize = DefaultPageSize
	}

	return req
}

// NewCursorPaginator creates a cursor paginator.
func NewCursorPaginator[T any](items []T, perPage int, hasMore bool) *CursorPaginator[T] {
	return &CursorPaginator[T]{
		items:   items,
		perPage: perPage,
		hasMore: hasMore,
	}
}

// Items returns the paginated items.
func (p *CursorPaginator[T]) Items() []T {
	return p.items
}

// HasMore returns true if there are more items.
func (p *CursorPaginator[T]) HasMore() bool {
	return p.hasMore
}

// NextCursor returns the cursor for the next page.
func (p *CursorPaginator[T]) NextCursor() *string {
	return p.nextCursor
}

// PrevCursor returns the cursor for the previous page.
func (p *CursorPaginator[T]) PrevCursor() *string {
	return p.prevCursor
}

// SetPath sets the base path for URL generation.
func (p *CursorPaginator[T]) SetPath(path string) *CursorPaginator[T] {
	p.path = path
	return p
}

// SetNextCursor sets the next page cursor.
func (p *CursorPaginator[T]) SetNextCursor(cursor *Cursor) {
	if cursor != nil {
		encoded := cursor.Encode()
		p.nextCursor = &encoded
	}
}

// SetPrevCursor sets the previous page cursor.
func (p *CursorPaginator[T]) SetPrevCursor(cursor *Cursor) {
	if cursor != nil {
		encoded := cursor.Encode()
		p.prevCursor = &encoded
	}
}

// ToMap converts to a map for JSON serialization.
func (p *CursorPaginator[T]) ToMap() map[string]any {
	result := map[string]any{
		"data":     p.items,
		"per_page": p.perPage,
		"has_more": p.hasMore,
	}

	if p.nextCursor != nil {
		result["next_cursor"] = *p.nextCursor
	}
	if p.prevCursor != nil {
		result["prev_cursor"] = *p.prevCursor
	}
	if p.path != "" {
		result["path"] = p.path
	}

	return result
}

// CursorPaginate performs cursor-based pagination.
//
// Parameters:
//   - db: GORM database instance
//   - req: Cursor request with cursor and per_page
//   - cursorField: Field to use for cursor (e.g., "created_at")
//   - order: Sort order ("asc" or "desc")
//   - idField: Primary key field name (default "id")
//
// Example:
//
//	items, paginator, err := pagination.CursorPaginate[Post](
//	    db.Where("user_id = ?", userID),
//	    req,
//	    "created_at",
//	    "desc",
//	    "id",
//	)
func CursorPaginate[T any](
	db *gorm.DB,
	req *CursorRequest,
	cursorField string,
	order string,
	idField string,
) ([]T, *CursorPaginator[T], error) {
	var items []T
	perPage := req.GetPageSize()

	// Build query
	query := db.Model(new(T))

	// Apply cursor if provided
	if req.Cursor != "" {
		cursor, err := DecodeCursor(req.Cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}

		// Build cursor condition based on order
		if order == "desc" {
			query = query.Where(
				fmt.Sprintf("(%s < ? OR (%s = ? AND %s < ?))", cursorField, cursorField, idField),
				cursor.Value, cursor.Value, cursor.ID,
			)
		} else {
			query = query.Where(
				fmt.Sprintf("(%s > ? OR (%s = ? AND %s > ?))", cursorField, cursorField, idField),
				cursor.Value, cursor.Value, cursor.ID,
			)
		}
	}

	// Order and limit (fetch one extra to check if there are more)
	orderClause := fmt.Sprintf("%s %s, %s %s", cursorField, order, idField, order)
	if err := query.Order(orderClause).Limit(perPage + 1).Find(&items).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to fetch items: %w", err)
	}

	// Check if there are more items
	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage] // Remove the extra item
	}

	paginator := NewCursorPaginator(items, perPage, hasMore)

	// Set next cursor if there are more items
	if hasMore && len(items) > 0 {
		lastItem := items[len(items)-1]
		nextCursor := buildCursor(lastItem, cursorField, idField, "next")
		paginator.SetNextCursor(nextCursor)
	}

	return items, paginator, nil
}

// buildCursor creates a cursor from an item.
func buildCursor[T any](item T, cursorField, idField, direction string) *Cursor {
	// Use reflection to get field values
	// This is a simplified version - in production you'd want more robust reflection
	cursor := &Cursor{
		Field:     cursorField,
		Direction: direction,
	}

	// For common cases, try to extract values
	// This works with structs that have ID and common timestamp fields
	type hasID interface{ GetID() uint }
	type hasCreatedAt interface{ GetCreatedAt() time.Time }

	if v, ok := any(item).(hasID); ok {
		cursor.ID = v.GetID()
	}

	if v, ok := any(item).(hasCreatedAt); ok {
		cursor.Value = v.GetCreatedAt()
	}

	return cursor
}

// NewCursor creates a cursor paginator from Gin context.
//
// Example:
//
//	posts, paginator, err := pagination.NewCursor[Post](c, db, "created_at", "desc")
func NewCursor[T any](c *gin.Context, db *gorm.DB, cursorField, order string) ([]T, *CursorPaginator[T], error) {
	req := CursorFromContext(c)
	items, paginator, err := CursorPaginate[T](db, req, cursorField, order, "id")
	if err != nil {
		return nil, nil, err
	}

	paginator.SetPath(c.Request.URL.Path)
	return items, paginator, nil
}
