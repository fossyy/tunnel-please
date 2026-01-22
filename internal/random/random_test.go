package random

import (
	"errors"
	"fmt"
	"io"
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
	errBrainrot := fmt.Errorf("you are not sigma enough")

	tests := []struct {
		name      string
		reader    io.Reader
		expectErr error
	}{
		{
			name:      "failing reader",
			reader:    &brainrotReader{err: errBrainrot},
			expectErr: errBrainrot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			randomizer := &random{reader: tt.reader}
			result, err := randomizer.String(20)
			if !errors.Is(err, tt.expectErr) {
				t.Errorf("String() error = %v, wantErr %v", err, tt.expectErr)
				return
			}

			if result != "" {
				t.Errorf("String() result = %v, want an empty string due to error", result)
			}
		})
	}
}
