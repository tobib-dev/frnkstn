package data

import (
	"errors"
	"slices"
	"strings"
)

type Filters struct {
	Page         int
	PageSize     int
	Sort         string
	SortSafeList []string
}

func ValidateFilters(f Filters) error {
	if f.Page < 1 || f.Page > 10_000_000 {
		return errors.New("page must be between 1 and 10000000")
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		return errors.New("page size must be between 1 and 100")
	}
	if !slices.Contains(f.SortSafeList, f.Sort) {
		return errors.New("sort method is not permitted")
	}
	return nil
}

func (f Filters) limit() int {
	return f.PageSize
}

func (f Filters) offset() int {
	return (f.Page - 1) * f.PageSize
}

func (f Filters) sortColumn() string {
	return strings.TrimPrefix(f.Sort, "-")
}

func (f Filters) sortDirection() string {
	if strings.HasPrefix(f.Sort, "-") {
		return "DESC"
	}
	return "ASC"
}
