package response

type Paginated[T any] struct {
	Items    []T `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

func PaginateItems[T any](items []T, page, pageSize int) Paginated[T] {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}

	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	paged := items[start:end]
	if paged == nil {
		paged = []T{}
	}

	return Paginated[T]{
		Items:    paged,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}
}
