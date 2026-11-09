// Package page implements the pagination contract shared by every list
// endpoint: ?page=1&pageSize=20 in, {items, page, pageSize, total} out.
package page

import (
	"errors"
	"net/http"
	"strconv"
)

const (
	// DefaultSize is used when pageSize is absent.
	DefaultSize = 20
	// MaxSize is the largest page a client may ask for.
	MaxSize = 100
	// maxPage keeps the offset inside int32 for the database.
	maxPage = 1_000_000
)

// ErrInvalid is returned for a malformed or out-of-range page or pageSize.
var ErrInvalid = errors.New("page must be an integer >= 1 and pageSize an integer between 1 and 100")

// Params is a validated page request.
type Params struct {
	Page int // 1-based
	Size int
}

// Default is the first page with the default size.
var Default = Params{Page: 1, Size: DefaultSize}

// Limit is the SQL LIMIT for this page.
func (p Params) Limit() int32 { return int32(p.Size) } //nolint:gosec // Size <= MaxSize

// Offset is the SQL OFFSET for this page.
func (p Params) Offset() int32 { return int32((p.Page - 1) * p.Size) } //nolint:gosec // Page <= maxPage, Size <= MaxSize

// FromRequest reads page and pageSize from the query string.
func FromRequest(r *http.Request) (Params, error) {
	p := Default
	q := r.URL.Query()
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPage {
			return p, ErrInvalid
		}
		p.Page = n
	}
	if v := q.Get("pageSize"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxSize {
			return p, ErrInvalid
		}
		p.Size = n
	}
	return p, nil
}

// Result is the JSON envelope of a list response.
type Result[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
}

// New builds a Result; a nil items slice is serialised as [].
func New[T any](items []T, p Params, total int64) Result[T] {
	if items == nil {
		items = []T{}
	}
	return Result[T]{Items: items, Page: p.Page, PageSize: p.Size, Total: total}
}

// Slice returns the window of all selected by p (used by in-memory repositories).
func Slice[T any](all []T, p Params) []T {
	start := (p.Page - 1) * p.Size
	if start >= len(all) {
		return []T{}
	}
	end := min(start+p.Size, len(all))
	return all[start:end]
}
