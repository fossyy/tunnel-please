package random

import (
	"errors"
	"fmt"
	"testing"
)

type brainrotReader struct {
	err error
}

func (f *brainrotReader) Read(p []byte) (int, error) {
	return 0, f.err
}

func TestRandom_String(t *testing.T) {
	tests := []struct {
		name    string
		length  int
		wantErr bool
	}{
		{"ValidLengthZero", 0, false},
		{"ValidPositiveLength", 10, false},
		{"NegativeLength", -1, true},
		{"VeryLargeLength", 1_000_000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			randomizer := New()

			result, err := randomizer.String(tt.length)
			if (err != nil) != tt.wantErr {
				t.Errorf("String() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(result) != tt.length {
				t.Errorf("String() length = %v, want %v", len(result), tt.length)
			}
		})
	}
}

func TestRandomWithFailingReader_String(t *testing.T) {
	var randomizer Random
	var errBrainrot = fmt.Errorf("you are not sigma enough")
	randomizer = &random{reader: &brainrotReader{err: errBrainrot}}
	t.Run("test failing reader", func(t *testing.T) {
		result, err := randomizer.String(20)
		if !errors.Is(err, errBrainrot) {
			t.Errorf("String() error = %v, wantErr %v", err, errBrainrot)
			return
		}

		if result != "" {
			t.Errorf("String() result = %v, want an empty string due to error", result)
		}
	})
}
