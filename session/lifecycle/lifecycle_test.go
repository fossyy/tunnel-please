package lifecycle

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"tunnel_pls/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/ssh"
)

type MockSessionRegistry struct {
	mock.Mock
}

func (m *MockSessionRegistry) Remove(key types.SessionKey) {
	m.Called(key)
}

type MockForwarder struct {
	mock.Mock
}

func (m *MockForwarder) CreateForwardedTCPIPPayload(origin net.Addr) []byte {
	args := m.Called(origin)
	return args.Get(0).([]byte)
}

func (m *MockForwarder) HandleConnection(dst io.ReadWriter, src ssh.Channel) {
	m.Called(dst, src)
}

func (m *MockForwarder) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockForwarder) TunnelType() types.TunnelType {
	args := m.Called()
	return args.Get(0).(types.TunnelType)
}

func (m *MockForwarder) ForwardedPort() uint16 {
	args := m.Called()
	return args.Get(0).(uint16)
}

func (m *MockForwarder) SetType(tunnelType types.TunnelType) {
	m.Called(tunnelType)
}

func (m *MockForwarder) SetForwardedPort(port uint16) {
	m.Called(port)
}

func (m *MockForwarder) SetListener(listener net.Listener) {
	m.Called(listener)
}

func (m *MockForwarder) Listener() net.Listener {
	args := m.Called()
	return args.Get(0).(net.Listener)
}

func (m *MockForwarder) OpenForwardedChannel(ctx context.Context, origin net.Addr) (ssh.Channel, <-chan *ssh.Request, error) {
	args := m.Called(ctx, origin)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(ssh.Channel), args.Get(1).(<-chan *ssh.Request), args.Error(2)
}

type MockPort struct {
	mock.Mock
}

func (m *MockPort) AddRange(startPort, endPort uint16) error {
	return m.Called(startPort, endPort).Error(0)
}
func (m *MockPort) Unassigned() (uint16, bool) {
	args := m.Called()
	var port uint16
	if args.Get(0) != nil {
		switch v := args.Get(0).(type) {
		case int:
			port = uint16(v)
		case uint16:
			port = v
		case uint32:
			port = uint16(v)
		case int32:
			port = uint16(v)
		case float64:
			port = uint16(v)
		default:
			port = uint16(args.Int(0))
		}
	}
	return port, args.Bool(1)
}
func (m *MockPort) SetStatus(port uint16, assigned bool) error {
	return m.Called(port, assigned).Error(0)
}
func (m *MockPort) Claim(port uint16) bool {
	return m.Called(port).Bool(0)
}

type MockSlug struct {
	mock.Mock
}

func (ms *MockSlug) Set(slug string) {
	ms.Called(slug)
}
func (ms *MockSlug) String() string {
	return ms.Called().String(0)
}

type MockSSHConn struct {
	ssh.Conn
	mock.Mock
}

func (m *MockSSHConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockSSHChannel struct {
	ssh.Channel
	mock.Mock
}

func (m *MockSSHChannel) Close() error {
	return m.Called().Error(0)
}

type mockNewChannel struct {
	ssh.NewChannel
	mock.Mock
}

func (m *mockNewChannel) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
	args := m.Called()
	return args.Get(0).(ssh.Channel), args.Get(1).(<-chan *ssh.Request), args.Error(2)
}

func (m *MockSSHConn) OpenChannel(name string, data []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	args := m.Called(name, data)
	if args.Get(0) == nil {
		return nil, args.Get(1).(<-chan *ssh.Request), args.Error(2)
	}
	return args.Get(0).(ssh.Channel), args.Get(1).(<-chan *ssh.Request), args.Error(2)
}

func TestNew(t *testing.T) {
	mockSSHConn := new(MockSSHConn)
	mockForwarder := &MockForwarder{}
	mockSlug := &MockSlug{}
	mockPort := &MockPort{}
	mockSessionRegistry := &MockSessionRegistry{}

	mockLifecycle := New(mockSSHConn, mockForwarder, mockSlug, mockPort, mockSessionRegistry, "mas-fuad")

	assert.NotNil(t, mockLifecycle.Connection())
	assert.NotNil(t, mockLifecycle.User())
	assert.NotNil(t, mockLifecycle.PortRegistry())
	assert.NotNil(t, mockLifecycle.StartedAt())
}

func TestLifecycle_User(t *testing.T) {
	mockSSHConn := new(MockSSHConn)
	mockForwarder := &MockForwarder{}
	mockSlug := &MockSlug{}
	mockPort := &MockPort{}
	mockSessionRegistry := &MockSessionRegistry{}

	user := "mas-fuad"
	mockLifecycle := New(mockSSHConn, mockForwarder, mockSlug, mockPort, mockSessionRegistry, user)
	assert.Equal(t, user, mockLifecycle.User())
}

func TestLifecycle_SetChannel(t *testing.T) {
	mockSSHConn := new(MockSSHConn)
	mockForwarder := &MockForwarder{}
	mockSlug := &MockSlug{}
	mockPort := &MockPort{}
	mockSessionRegistry := &MockSessionRegistry{}

	mockLifecycle := New(mockSSHConn, mockForwarder, mockSlug, mockPort, mockSessionRegistry, "mas-fuad")

	mockSSHChannel := &MockSSHChannel{}

	mockLifecycle.SetChannel(mockSSHChannel)

	assert.Equal(t, mockSSHChannel, mockLifecycle.Channel())
}

func TestLifecycle_SetStatus(t *testing.T) {
	mockSSHConn := new(MockSSHConn)
	mockForwarder := &MockForwarder{}
	mockSlug := &MockSlug{}
	mockPort := &MockPort{}
	mockSessionRegistry := &MockSessionRegistry{}

	mockLifecycle := New(mockSSHConn, mockForwarder, mockSlug, mockPort, mockSessionRegistry, "mas-fuad")

	mockLifecycle.SetStatus(types.SessionStatusRUNNING)
	assert.True(t, mockLifecycle.IsActive())
}

func TestLifecycle_IsActive(t *testing.T) {
	mockSSHConn := new(MockSSHConn)
	mockForwarder := &MockForwarder{}
	mockSlug := &MockSlug{}
	mockPort := &MockPort{}
	mockSessionRegistry := &MockSessionRegistry{}

	mockLifecycle := New(mockSSHConn, mockForwarder, mockSlug, mockPort, mockSessionRegistry, "mas-fuad")

	mockLifecycle.SetStatus(types.SessionStatusRUNNING)
	assert.True(t, mockLifecycle.IsActive())
}

func TestLifecycle_Close(t *testing.T) {
	tests := []struct {
		name            string
		tunnelType      types.TunnelType
		connCloseErr    error
		channelCloseErr error
		expectErr       bool
		alreadyClosed   bool
	}{
		{
			name:       "Close HTTP forwarding success",
			tunnelType: types.TunnelTypeHTTP,
			expectErr:  false,
		},
		{
			name:       "Close TCP forwarding success",
			tunnelType: types.TunnelTypeTCP,
			expectErr:  false,
		},
		{
			name:         "Close with conn close error",
			tunnelType:   types.TunnelTypeHTTP,
			connCloseErr: errors.New("conn close error"),
			expectErr:    true,
		},
		{
			name:            "Close with channel close error",
			tunnelType:      types.TunnelTypeHTTP,
			channelCloseErr: errors.New("channel close error"),
			expectErr:       true,
		},
		{
			name:          "Close when already closed",
			tunnelType:    types.TunnelTypeHTTP,
			alreadyClosed: true,
			expectErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSSHConn := &MockSSHConn{}
			mockSSHConn.On("Close").Return(tt.connCloseErr)

			mockForwarder := &MockForwarder{}
			mockForwarder.On("TunnelType").Return(tt.tunnelType)
			if tt.tunnelType == types.TunnelTypeTCP {
				mockForwarder.On("ForwardedPort").Return(uint16(8080))
				mockForwarder.On("Close").Return(nil)
			}

			mockSlug := &MockSlug{}
			mockSlug.On("String").Return("test-slug")

			mockPort := &MockPort{}
			if tt.tunnelType == types.TunnelTypeTCP {
				mockPort.On("SetStatus", uint16(8080), false).Return(nil)
			}

			mockSessionRegistry := &MockSessionRegistry{}
			mockSessionRegistry.On("Remove", mock.Anything).Return()

			mockSSHChannel := &MockSSHChannel{}
			mockSSHChannel.On("Close").Return(tt.channelCloseErr)

			mockLifecycle := New(mockSSHConn, mockForwarder, mockSlug, mockPort, mockSessionRegistry, "mas-fuad")

			mockLifecycle.SetStatus(types.SessionStatusRUNNING)
			mockLifecycle.SetChannel(mockSSHChannel)

			if tt.alreadyClosed {
				mockLifecycle.Close()
			}

			err := mockLifecycle.Close()

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.False(t, mockLifecycle.IsActive())

			mockSSHConn.AssertExpectations(t)
			mockForwarder.AssertExpectations(t)
			mockSlug.AssertExpectations(t)
			mockPort.AssertExpectations(t)
			mockSessionRegistry.AssertExpectations(t)
			mockSSHChannel.AssertExpectations(t)
		})
	}
}
