package random

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.length)
			}
		})
	}
}

func TestRandomWithFailingReader_String(t *testing.T) {
	errBrainrot := assert.AnError

	tests := []struct {
		name      string
		reader    io.Reader
		expectErr error
	}{
		{
			name: "failing reader",
			reader: func() io.Reader {
				return &failingReader{err: errBrainrot}
			}(),
			expectErr: errBrainrot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			randomizer := &random{reader: tt.reader}
			result, err := randomizer.String(20)
			assert.ErrorIs(t, err, tt.expectErr)
			assert.Empty(t, result)
		})
	}
}

type failingReader struct {
	err error
}

func (f *failingReader) Read(p []byte) (int, error) {
	return 0, f.err
}
