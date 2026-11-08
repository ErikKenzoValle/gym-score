package repositories

import "testing"

func TestReaisBR(t *testing.T) {
	casos := map[int64]string{0: "R$ 0,00", 5: "R$ 0,05", 123456: "R$ 1.234,56", 100000000: "R$ 1.000.000,00", -250: "-R$ 2,50"}
	for c, want := range casos {
		if got := ReaisBR(c); got != want {
			t.Errorf("ReaisBR(%d) = %q, quer %q", c, got, want)
		}
	}
}
