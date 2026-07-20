package claims

import "testing"

// Adversarial probes for coverage soundness.
func TestReviewProbes(t *testing.T) {
	// Prefix hole: does allowed "10000 developers" wrongly cover claimed "1000"?
	if v := Check("serving 1000 developers", []string{"10000 developers"}); len(v) != 1 {
		t.Errorf("PREFIX HOLE: allowed 10000 covered claimed 1000: %+v", v)
	}
	// Suffix at start-of-string: allowed "1000+ x" vs claimed "1000+" is legit.
	if v := Check("1000+ developers", []string{"1000+ anything"}); v != nil {
		t.Errorf("start-of-string legit match failed: %+v", v)
	}
}
