package pagination

import (
	"github.com/gin-gonic/gin"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100
)

// Params holds the parsed pagination parameters from a request.
type Params struct {
	Page     int `form:"page"      json:"page"`
	PageSize int `form:"page_size" json:"page_size"`
}

// Result is the standard paginated response envelope.
// Use the generic New function to construct it.
type Result[T any] struct {
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Data     []T   `json:"data"`
}

// FromQuery parses pagination params from the gin query string.
// Applies defaults (page=1, page_size=20) and caps page_size at 100.
func FromQuery(c *gin.Context) Params {
	var p Params
	_ = c.ShouldBindQuery(&p)
	return normalize(p)
}

// FromParams creates Params from explicit page and pageSize values,
// applying defaults and limits. Useful outside of gin handlers.
func FromParams(page, pageSize int) Params {
	return normalize(Params{Page: page, PageSize: pageSize})
}

// Offset returns the database query offset for the given Params.
func Offset(p Params) int {
	return (p.Page - 1) * p.PageSize
}

// New constructs a Result from total record count, data slice, and Params.
func New[T any](total int64, data []T, p Params) Result[T] {
	if data == nil {
		data = []T{}
	}
	return Result[T]{
		Total:    total,
		Page:     p.Page,
		PageSize: p.PageSize,
		Data:     data,
	}
}

func normalize(p Params) Params {
	if p.Page < 1 {
		p.Page = defaultPage
	}
	if p.PageSize < 1 {
		p.PageSize = defaultPageSize
	}
	if p.PageSize > maxPageSize {
		p.PageSize = maxPageSize
	}
	return p
}
