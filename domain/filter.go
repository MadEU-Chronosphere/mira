package domain

// Add to any filter struct that's used for list endpoints
type PaginationFilter struct {
	Page  int // 1-based; default 1
	Limit int // default 20, max 100
}

func (f *PaginationFilter) Offset() int {
	if f.Page < 1 {
		f.Page = 1
	}
	return (f.Page - 1) * f.Limit
}

func (f *PaginationFilter) SafeLimit() int {
	if f.Limit <= 0 || f.Limit > 100 {
		return 20
	}
	return f.Limit
}