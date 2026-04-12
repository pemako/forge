// Package pagination provides helpers for cursor-free, page-number-based
// pagination in gin HTTP handlers. It parses and normalises page/page_size
// query parameters, enforces sensible defaults and an upper bound, and wraps
// query results in a standard envelope ready for JSON serialisation.
//
// Typical usage:
//
//	func ListUsers(c *gin.Context) {
//	    p := pagination.FromQuery(c)
//	    users, total, err := svc.ListUsers(ctx, pagination.Offset(p), p.PageSize)
//	    c.JSON(200, pagination.New(total, users, p))
//	}
package pagination

import (
	"github.com/gin-gonic/gin"
)

const (
	// defaultPage is the page number applied when the caller omits page or
	// supplies a value less than 1.
	defaultPage = 1

	// defaultPageSize is the number of items per page applied when the caller
	// omits page_size or supplies a value less than 1.
	defaultPageSize = 20

	// maxPageSize is the hard upper bound on page_size. Requests that exceed
	// this value are silently capped to prevent runaway queries.
	maxPageSize = 100
)

// Params holds the parsed and normalised pagination parameters extracted from a
// request. Construct it via FromQuery (gin handler) or FromParams (service
// layer). Both apply defaults and enforce the maxPageSize cap.
type Params struct {
	// Page is the 1-based page number. Defaults to 1 when absent or < 1.
	Page int `form:"page"      json:"page"`
	// PageSize is the number of items per page. Defaults to 20 when absent or
	// < 1, capped at 100 regardless of the requested value.
	PageSize int `form:"page_size" json:"page_size"`
}

// Result is the standard paginated response envelope returned to API callers.
// Use New to construct it; the zero value is not meaningful.
type Result[T any] struct {
	// Total is the total number of matching records across all pages, as
	// reported by the underlying data store.
	Total int64 `json:"total"`
	// Page mirrors Params.Page of the request that produced this result.
	Page int `json:"page"`
	// PageSize mirrors Params.PageSize of the request that produced this result.
	PageSize int `json:"page_size"`
	// Data holds the items on the current page. It is never nil; an empty page
	// is represented as an empty slice so JSON serialises to [].
	Data []T `json:"data"`
}

// FromQuery parses pagination parameters from the gin request's query string
// (page and page_size). Missing or invalid values are replaced with defaults
// (page=1, page_size=20); page_size is capped at 100. See also FromParams for
// use outside gin handlers.
func FromQuery(c *gin.Context) Params {
	var p Params
	_ = c.ShouldBindQuery(&p)
	return normalize(p)
}

// FromParams constructs a Params from explicit page and pageSize integers,
// applying the same defaults and cap as FromQuery. Use this in service-layer
// code or tests where no gin.Context is available.
func FromParams(page, pageSize int) Params {
	return normalize(Params{Page: page, PageSize: pageSize})
}

// Offset returns the zero-based row offset to pass to a database LIMIT/OFFSET
// query for the given Params. Use alongside Params.PageSize as the LIMIT value.
//
//	db.Offset(pagination.Offset(p)).Limit(p.PageSize)
func Offset(p Params) int {
	return (p.Page - 1) * p.PageSize
}

// New constructs a Result from the total record count, the current-page data
// slice, and the Params that drove the query. If data is nil it is replaced
// with an empty slice so the JSON response always contains an array.
// Pair with FromQuery and Offset for a complete pagination flow.
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
