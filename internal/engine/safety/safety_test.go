package safety

import "testing"

func TestResultZeroValueIsSafe(t *testing.T) {
	var r Result
	if r.Unsafe {
		t.Fatalf("zero-value Result.Unsafe = true, want false")
	}
	if len(r.Statements) != 0 {
		t.Fatalf("zero-value Result.Statements = %+v, want empty", r.Statements)
	}
}

func TestUnsafeStatementFields(t *testing.T) {
	s := UnsafeStatement{Kind: KindUnsafeMissingWhere, StartOffset: 3, EndOffset: 9}
	if s.Kind != KindUnsafeMissingWhere || s.StartOffset != 3 || s.EndOffset != 9 {
		t.Fatalf("unexpected UnsafeStatement: %+v", s)
	}
}
