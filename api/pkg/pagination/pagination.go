// Package pagination provides unified pagination for the ZGI API.
//
// Two pagination styles are supported:
//   - Page-based: PageRequest + Paginator[T] for standard list APIs
//   - Cursor-based: CursorPaginator[T] for infinite scroll (see cursor.go)
//
// Usage in DTO (embed PageRequest):
//
//	type ListUsersRequest struct {
//	    pagination.PageRequest
//	    Keyword string `form:"keyword"`
//	}
//
// Usage in Handler (GORM integration):
//
//	req := pagination.FromContext(c)
//	items, paginator, err := pagination.Paginate[User](db.Where("active = ?", true), req)
//	c.JSON(200, paginator)
//
// Usage in Service (manual):
//
//	paginator := pagination.NewPaginator(items, total, page, pageSize)
package pagination

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ============================================================================
// Constants
// ============================================================================

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ============================================================================
// Request - Embeddable struct for DTOs
// ============================================================================

// PageRequest is the standard pagination request struct.
// Embed this in your DTO for unified pagination parameters.
//
// Example:
//
//	type ListUsersRequest struct {
//	    pagination.PageRequest
//	    Keyword string `form:"keyword"`
//	}
type PageRequest struct {
	Page     int    `form:"page" json:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" json:"page_size" binding:"omitempty,min=1,max=100"`
	Sort     string `form:"sort" json:"sort,omitempty"`
	Order    string `form:"order" json:"order,omitempty"`
}

// GetPage returns the page number with default value.
func (p *PageRequest) GetPage() int {
	if p.Page <= 0 {
		return DefaultPage
	}
	return p.Page
}

// GetPageSize returns the page size with default value.
func (p *PageRequest) GetPageSize() int {
	if p.PageSize <= 0 {
		return DefaultPageSize
	}
	if p.PageSize > MaxPageSize {
		return MaxPageSize
	}
	return p.PageSize
}

// GetOffset calculates the SQL offset for the current page.
func (p *PageRequest) GetOffset() int {
	return (p.GetPage() - 1) * p.GetPageSize()
}

// GetSort returns the sort field, or empty string if not set.
func (p *PageRequest) GetSort() string {
	return p.Sort
}

// GetOrder returns the sort order, defaults to "desc".
func (p *PageRequest) GetOrder() string {
	if p.Order == "asc" {
		return "asc"
	}
	return "desc"
}

// GetOrderBy returns the full ORDER BY clause.
// Returns empty string if no sort field is specified.
func (p *PageRequest) GetOrderBy() string {
	if p.Sort == "" {
		return ""
	}
	return p.Sort + " " + p.GetOrder()
}

// FromContext extracts pagination parameters from Gin context query string.
func FromContext(c *gin.Context) *PageRequest {
	req := &PageRequest{}
	if page, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil {
		req.Page = page
	}
	if pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(DefaultPageSize))); err == nil {
		req.PageSize = pageSize
	}
	req.Sort = c.Query("sort")
	req.Order = c.DefaultQuery("order", "desc")
	return req
}

// ============================================================================
// Paginator[T] - Core paginated result
// ============================================================================

// Paginator holds pagination state and items.
type Paginator[T any] struct {
	Items      []T   `json:"data"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// NewPaginator creates a new paginator instance.
func NewPaginator[T any](items []T, total int64, page, pageSize int) *Paginator[T] {
	if page < 1 {
		page = DefaultPage
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	if totalPages < 1 {
		totalPages = 1
	}

	return &Paginator[T]{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}
}

// ============================================================================
// GORM Integration
// ============================================================================

// Paginate executes a paginated query on GORM.
//
// Example:
//
//	req := pagination.FromContext(c)
//	items, paginator, err := pagination.Paginate[User](db.Where("active = ?", true), req)
func Paginate[T any](db *gorm.DB, req *PageRequest) ([]T, *Paginator[T], error) {
	var items []T
	var total int64

	countDB := db.Session(&gorm.Session{})
	if err := countDB.Count(&total).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to count: %w", err)
	}

	offset := req.GetOffset()
	if err := db.Offset(offset).Limit(req.GetPageSize()).Find(&items).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to fetch items: %w", err)
	}

	paginator := NewPaginator(items, total, req.GetPage(), req.GetPageSize())
	return items, paginator, nil
}

// PaginateWithScope executes a paginated query from Gin context with a scope function.
//
// Example:
//
//	items, paginator, err := pagination.PaginateWithScope[User](c, db, func(db *gorm.DB) *gorm.DB {
//	    return db.Where("status = ?", "active").Order("created_at DESC")
//	})
func PaginateWithScope[T any](c *gin.Context, db *gorm.DB, scope func(*gorm.DB) *gorm.DB) ([]T, *Paginator[T], error) {
	req := FromContext(c)
	return Paginate[T](scope(db.Model(new(T))), req)
}

// ============================================================================
// Service Layer Helper
// ============================================================================

// Result wraps paginated data for service layer returns.
//
// Example (Service):
//
//	func (s *service) List(ctx context.Context, page, pageSize int) (*pagination.Result[*User], error) {
//	    users, total, err := s.repo.FindAll(ctx, page, pageSize)
//	    return pagination.NewResult(users, total, page, pageSize), nil
//	}
type Result[T any] struct {
	Items    []T
	Total    int64
	Page     int
	PageSize int
}

// NewResult creates a pagination result from service layer.
func NewResult[T any](items []T, total int64, page, pageSize int) *Result[T] {
	return &Result[T]{Items: items, Total: total, Page: page, PageSize: pageSize}
}

// ToPaginator converts Result to Paginator.
func (r *Result[T]) ToPaginator() *Paginator[T] {
	return NewPaginator(r.Items, r.Total, r.Page, r.PageSize)
}

// ============================================================================
// InfiniteScrollPagination - For cursor/scroll style APIs
// ============================================================================

// InfiniteScrollPagination is a simple response for infinite scroll APIs.
type InfiniteScrollPagination struct {
	Limit   int         `json:"limit"`
	HasMore bool        `json:"has_more"`
	Data    interface{} `json:"data"`
}

// NewInfiniteScrollPagination creates a new infinite scroll pagination response.
func NewInfiniteScrollPagination(data interface{}, limit int, hasMore bool) *InfiniteScrollPagination {
	return &InfiniteScrollPagination{
		Limit:   limit,
		HasMore: hasMore,
		Data:    data,
	}
}
