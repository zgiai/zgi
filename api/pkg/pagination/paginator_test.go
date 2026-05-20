package pagination

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPaginator(t *testing.T) {
	items := []string{"a", "b", "c"}
	p := NewPaginator(items, 100, 1, 20)

	assert.Equal(t, 3, len(p.Items))
	assert.Equal(t, int64(100), p.Total)
	assert.Equal(t, 1, p.Page)
	assert.Equal(t, 20, p.PageSize)
	assert.Equal(t, 5, p.TotalPages)
	assert.True(t, p.HasNext)
	assert.False(t, p.HasPrev)
}

func TestPaginatorBounds(t *testing.T) {
	items := []string{"a"}

	// Test page < 1
	p := NewPaginator(items, 100, 0, 20)
	assert.Equal(t, 1, p.Page)

	// Test pageSize < 1
	p = NewPaginator(items, 100, 1, 0)
	assert.Equal(t, DefaultPageSize, p.PageSize)

	// Test pageSize > max
	p = NewPaginator(items, 100, 1, 200)
	assert.Equal(t, MaxPageSize, p.PageSize)
}

func TestPaginatorNavigation(t *testing.T) {
	items := []string{"a"}

	// First page
	p := NewPaginator(items, 100, 1, 20)
	assert.False(t, p.HasPrev)
	assert.True(t, p.HasNext)

	// Last page
	p = NewPaginator(items, 100, 5, 20)
	assert.True(t, p.HasPrev)
	assert.False(t, p.HasNext)

	// Single page
	p = NewPaginator(items, 10, 1, 20)
	assert.False(t, p.HasPrev)
	assert.False(t, p.HasNext)
	assert.Equal(t, 1, p.TotalPages)
}

func TestPaginatorTotalPages(t *testing.T) {
	items := []string{"a"}

	// Exact division
	p := NewPaginator(items, 100, 1, 20)
	assert.Equal(t, 5, p.TotalPages)

	// Remainder
	p = NewPaginator(items, 101, 1, 20)
	assert.Equal(t, 6, p.TotalPages)

	// Zero total
	p = NewPaginator([]string{}, 0, 1, 20)
	assert.Equal(t, 1, p.TotalPages)
}

func TestPageRequest(t *testing.T) {
	req := &PageRequest{Page: 2, PageSize: 20}

	assert.Equal(t, 2, req.GetPage())
	assert.Equal(t, 20, req.GetPageSize())
	assert.Equal(t, 20, req.GetOffset())

	// Defaults
	req = &PageRequest{}
	assert.Equal(t, DefaultPage, req.GetPage())
	assert.Equal(t, DefaultPageSize, req.GetPageSize())

	// Bounds
	req = &PageRequest{Page: -1, PageSize: 200}
	assert.Equal(t, DefaultPage, req.GetPage())
	assert.Equal(t, MaxPageSize, req.GetPageSize())
}

func TestPageRequestSort(t *testing.T) {
	req := &PageRequest{Sort: "created_at", Order: "asc"}
	assert.Equal(t, "created_at asc", req.GetOrderBy())

	req = &PageRequest{Sort: "created_at", Order: "desc"}
	assert.Equal(t, "created_at desc", req.GetOrderBy())

	req = &PageRequest{Sort: "created_at"} // No order specified
	assert.Equal(t, "created_at desc", req.GetOrderBy())

	req = &PageRequest{} // No sort
	assert.Equal(t, "", req.GetOrderBy())
}

func TestResult(t *testing.T) {
	items := []string{"a", "b", "c"}
	result := NewResult(items, 100, 2, 20)

	p := result.ToPaginator()
	assert.Equal(t, 3, len(p.Items))
	assert.Equal(t, int64(100), p.Total)
	assert.Equal(t, 2, p.Page)
	assert.Equal(t, 20, p.PageSize)
	assert.Equal(t, 5, p.TotalPages)
}
