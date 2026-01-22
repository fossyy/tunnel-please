package transport

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransportInterface(t *testing.T) {
	var _ Transport = (*httpServer)(nil)
	var _ Transport = (*https)(nil)
	var _ Transport = (*tcp)(nil)

	assert.True(t, true)
}
