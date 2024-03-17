package sizedbuf

import (
	"bytes"
	"testing"
)

func TestSizedBuffer_Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		inputs            []string
		limit             int
		expectedSizeAfter int
		shouldFlush       bool
	}{
		{"test_small_write", []string{"Hello"}, 10, 5, false},
		{"test_exact_limit", []string{"HelloWorld"}, 10, 0, true},
		{"test_over_limit", []string{"HelloWorld!"}, 10, 0, true},
		{"test_multiple_writes", []string{"Hi", "Hello", "hey"}, 5, 3, true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(
			tt.name, func(t *testing.T) {
				t.Parallel()

				var buf bytes.Buffer
				sb := New(&buf, tt.limit)

				for _, input := range tt.inputs {
					_, err := sb.Write([]byte(input))
					if err != nil {
						t.Errorf("Write() error = %v", err)
					}
				}

				if sb.size != tt.expectedSizeAfter {
					t.Errorf("Expected size after write = %d, got %d", tt.expectedSizeAfter, sb.size)
				}
			},
		)
	}
}
