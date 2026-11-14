package store

import "testing"

// Pure-function tests only; DB access is covered manually against pgvector.
func TestIsDuplicateThreshold(t *testing.T) {
	// Observed on real data: paraphrases of one fact scored 0.27-0.40,
	// clearly distinct facts scored 0.60+.
	for _, d := range []float64{0.0, 0.27, 0.399, 0.44} {
		if !IsDuplicate(d) {
			t.Fatalf("dist %v should count as duplicate", d)
		}
	}
	for _, d := range []float64{0.45, 0.6, 1.0, 2.0} {
		if IsDuplicate(d) {
			t.Fatalf("dist %v should count as distinct", d)
		}
	}
}
