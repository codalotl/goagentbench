package sorting_test

import (
	"fmt"
	"testing"
)

//
// Total tests: 6 + 10 + 1 = 17 (3 passes, 3 fails, 10 iterations of multi, and 1 multi itself (the parent))
// Passes: 3 + 3 (TestPass1-3, 3 t.Run in TestMulti. NOTE: TestMulti itself does NOT pass, since it has failing children)
// Partial: 0.3529..
//

func TestPass1(t *testing.T) {}
func TestPass2(t *testing.T) {}
func TestPass3(t *testing.T) {}

func TestFail1(t *testing.T) {
	t.Fatalf("failed")
}

func TestFail2(t *testing.T) {
	t.Fatalf("failed")
}

func TestFail3(t *testing.T) {
	t.Fatalf("failed")
}

func TestMulti(t *testing.T) {

	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("multi %d", i), func(t *testing.T) {
			if i >= 3 {
				t.Fatalf("fail multi %d", i)
			}
		})

	}
}
