package id

import "testing"

func TestNewIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		value := New("test")
		if value == "" || seen[value] {
			t.Fatalf("duplicate or empty id %q", value)
		}
		seen[value] = true
	}
}
