package sorting_test

import (
	"testing"

	"github.com/codalotl/goagentbench/internal/sorting"
)

// To test allow-multiple-turns-on-failed-verify, uncomment the boom:
// func TestBoom(t *testing.T) {
// 	t.Fatalf("an explosion occurred. It was not caused by you. Sorry. Just end yoru turn again.")
// }

func TestIsSorted(t *testing.T) {
	tests := []struct {
		name string
		nums []int
		want bool
	}{
		{
			name: "empty slice",
			nums: nil,
			want: true,
		},
		{
			name: "single element",
			nums: []int{42},
			want: true,
		},
		{
			name: "strictly increasing",
			nums: []int{1, 2, 3, 4, 5},
			want: true,
		},
		{
			name: "non-decreasing with duplicates",
			nums: []int{1, 1, 2, 2, 3, 3},
			want: true,
		},
		{
			name: "all equal",
			nums: []int{7, 7, 7, 7},
			want: true,
		},
		{
			name: "decreasing",
			nums: []int{5, 4, 3, 2, 1},
			want: false,
		},
		{
			name: "unsorted in the middle",
			nums: []int{1, 2, 4, 3, 5},
			want: false,
		},
		{
			name: "unsorted with duplicates",
			nums: []int{1, 1, 2, 1},
			want: false,
		},
		{
			name: "negatives sorted",
			nums: []int{-5, -3, -3, -1, 0, 2},
			want: true,
		},
		{
			name: "negatives unsorted",
			nums: []int{-5, -3, -4, -1},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			if got := sorting.IsSorted(tt.nums); got != tt.want {
				t.Fatalf("IsSorted(%v) = %v, want %v", tt.nums, got, tt.want)
			}
		})
	}
}
