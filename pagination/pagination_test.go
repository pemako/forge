package pagination_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/pemako/mk/pagination"
)

func TestFromParams_defaults(t *testing.T) {
	p := pagination.FromParams(0, 0)
	assert.Equal(t, 1, p.Page)
	assert.Equal(t, 20, p.PageSize)
}

func TestFromParams_maxPageSize(t *testing.T) {
	p := pagination.FromParams(1, 9999)
	assert.Equal(t, 100, p.PageSize)
}

func TestOffset(t *testing.T) {
	assert.Equal(t, 0, pagination.Offset(pagination.FromParams(1, 20)))
	assert.Equal(t, 20, pagination.Offset(pagination.FromParams(2, 20)))
	assert.Equal(t, 40, pagination.Offset(pagination.FromParams(3, 20)))
}

func TestNew_nilData(t *testing.T) {
	p := pagination.FromParams(1, 20)
	r := pagination.New[string](0, nil, p)
	assert.Equal(t, []string{}, r.Data) // never nil
}

func TestNew_withData(t *testing.T) {
	p := pagination.FromParams(2, 10)
	r := pagination.New[int](100, []int{1, 2, 3}, p)
	assert.Equal(t, int64(100), r.Total)
	assert.Equal(t, 2, r.Page)
	assert.Equal(t, 10, r.PageSize)
	assert.Len(t, r.Data, 3)
}

func TestFromQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		p := pagination.FromQuery(c)
		assert.Equal(t, 3, p.Page)
		assert.Equal(t, 15, p.PageSize)
		c.Status(200)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/?page=3&page_size=15", nil)
	r.ServeHTTP(w, req)
}
