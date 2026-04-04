package response

import "testing"

func TestPaginateItemsDefaultsAndEmptySlice(t *testing.T) {
	t.Parallel()

	result := PaginateItems[string](nil, 0, 0)

	if result.Page != 1 {
		t.Fatalf("expected default page 1, got %d", result.Page)
	}
	if result.PageSize != 25 {
		t.Fatalf("expected default page_size 25, got %d", result.PageSize)
	}
	if result.Total != 0 {
		t.Fatalf("expected total 0, got %d", result.Total)
	}
	if result.Items == nil {
		t.Fatal("expected empty items slice, got nil")
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected no items, got %d", len(result.Items))
	}
}

func TestPaginateItemsReturnsRequestedWindow(t *testing.T) {
	t.Parallel()

	result := PaginateItems([]int{1, 2, 3, 4, 5}, 2, 2)

	if result.Page != 2 {
		t.Fatalf("expected page 2, got %d", result.Page)
	}
	if result.PageSize != 2 {
		t.Fatalf("expected page_size 2, got %d", result.PageSize)
	}
	if result.Total != 5 {
		t.Fatalf("expected total 5, got %d", result.Total)
	}
	if len(result.Items) != 2 || result.Items[0] != 3 || result.Items[1] != 4 {
		t.Fatalf("unexpected items: %#v", result.Items)
	}
}
