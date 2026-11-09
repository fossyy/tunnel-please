package interaction

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCrlfWriterTranslatesBareNewlines(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		count int
	}{
		{
			name:  "bare lf becomes crlf",
			in:    "a\nb",
			want:  "a\r\nb",
			count: 3,
		},
		{
			name:  "existing crlf is not doubled",
			in:    "a\r\nb",
			want:  "a\r\nb",
			count: 4,
		},
		{
			name:  "leading lf becomes crlf",
			in:    "\na",
			want:  "\r\na",
			count: 2,
		},
		{
			name:  "no newline is untouched",
			in:    "abc",
			want:  "abc",
			count: 3,
		},
		{
			name:  "cursor down is not a newline",
			in:    "\x1b[2B",
			want:  "\x1b[2B",
			count: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n, err := crlfWriter{dst: &buf}.Write([]byte(tt.in))

			assert.NoError(t, err)
			assert.Equal(t, []byte(tt.want), buf.Bytes())
			assert.Equal(t, len(tt.in), n, "must report the caller's byte count")
		})
	}
}
