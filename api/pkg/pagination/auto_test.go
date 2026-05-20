package pagination

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func createTestContext(path string, query string) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{
		URL: &url.URL{Path: path, RawQuery: query},
	}
	return c
}

func TestFromContext(t *testing.T) {
	c := createTestContext("/api/users", "page=3&page_size=25&sort=name&order=asc")
	req := FromContext(c)

	assert.Equal(t, 3, req.Page)
	assert.Equal(t, 25, req.PageSize)
	assert.Equal(t, "name", req.Sort)
	assert.Equal(t, "asc", req.Order)
}

func TestFromContext_Defaults(t *testing.T) {
	c := createTestContext("/api/users", "")
	req := FromContext(c)

	assert.Equal(t, DefaultPage, req.GetPage())
	assert.Equal(t, DefaultPageSize, req.GetPageSize())
	assert.Equal(t, "desc", req.GetOrder())
}

func TestResult_NilItems(t *testing.T) {
	var nilItems []*string
	result := NewResult(nilItems, 0, 1, 20)

	assert.NotNil(t, result)
	assert.Nil(t, result.Items)
	assert.Equal(t, int64(0), result.Total)

	p := result.ToPaginator()
	assert.NotNil(t, p)
	assert.Equal(t, 0, len(p.Items))
}

func TestResult_EmptyItems(t *testing.T) {
	items := make([]*string, 0)
	result := NewResult(items, 0, 1, 20)

	p := result.ToPaginator()
	assert.NotNil(t, p)
	assert.Equal(t, 0, len(p.Items))
	assert.Equal(t, int64(0), p.Total)
}

func TestResult_InvalidPage(t *testing.T) {
	items := []string{"a", "b", "c"}
	result := NewResult(items, 100, 0, 20)

	p := result.ToPaginator()
	assert.Equal(t, 1, p.Page)
}

func TestResult_InvalidPageSize(t *testing.T) {
	items := []string{"a", "b", "c"}
	result := NewResult(items, 100, 1, 0)

	p := result.ToPaginator()
	assert.Equal(t, DefaultPageSize, p.PageSize)
}

func TestResult_ExceedsMaxPageSize(t *testing.T) {
	items := []string{"a", "b", "c"}
	result := NewResult(items, 100, 1, 1000)

	p := result.ToPaginator()
	assert.Equal(t, MaxPageSize, p.PageSize)
}

func TestPaginator_EmptyTotal(t *testing.T) {
	p := NewPaginator([]string{}, 0, 1, 20)

	assert.Equal(t, int64(0), p.Total)
	assert.Equal(t, 1, p.TotalPages)
	assert.False(t, p.HasNext)
	assert.False(t, p.HasPrev)
}
