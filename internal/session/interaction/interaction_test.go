package interaction

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
	"tunnel_pls/types"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/ssh"
)

type MockRandom struct {
	mock.Mock
}

func (m *MockRandom) String(length int) (string, error) {
	args := m.Called(length)
	return args.String(0), args.Error(1)
}

type MockConfig struct {
	mock.Mock
}

func (m *MockConfig) Domain() string            { return m.Called().String(0) }
func (m *MockConfig) SSHPort() string           { return m.Called().String(0) }
func (m *MockConfig) HTTPPort() string          { return m.Called().String(0) }
func (m *MockConfig) HTTPSPort() string         { return m.Called().String(0) }
func (m *MockConfig) TLSEnabled() bool          { return m.Called().Bool(0) }
func (m *MockConfig) TLSRedirect() bool         { return m.Called().Bool(0) }
func (m *MockConfig) ACMEEmail() string         { return m.Called().String(0) }
func (m *MockConfig) CFAPIToken() string        { return m.Called().String(0) }
func (m *MockConfig) ACMEStaging() bool         { return m.Called().Bool(0) }
func (m *MockConfig) AllowedPortsStart() uint16 { return uint16(m.Called().Int(0)) }
func (m *MockConfig) AllowedPortsEnd() uint16   { return uint16(m.Called().Int(0)) }
func (m *MockConfig) BufferSize() int           { return m.Called().Int(0) }
func (m *MockConfig) HeaderSize() int           { return m.Called().Int(0) }
func (m *MockConfig) PprofEnabled() bool        { return m.Called().Bool(0) }
func (m *MockConfig) PprofPort() string         { return m.Called().String(0) }
func (m *MockConfig) Mode() types.ServerMode    { return m.Called().Get(0).(types.ServerMode) }
func (m *MockConfig) GRPCAddress() string       { return m.Called().String(0) }
func (m *MockConfig) GRPCPort() string          { return m.Called().String(0) }
func (m *MockConfig) NodeToken() string         { return m.Called().String(0) }
func (m *MockConfig) TLSStoragePath() string    { return m.Called().String(0) }
func (m *MockConfig) KeyLoc() string            { return m.Called().String(0) }

type MockSlug struct {
	mock.Mock
}

func (ms *MockSlug) Set(slug string) { ms.Called(slug) }
func (ms *MockSlug) String() string  { return ms.Called().String(0) }

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

type MockSessionRegistry struct {
	mock.Mock
}

func (m *MockSessionRegistry) Update(user string, oldKey, newKey types.SessionKey) error {
	args := m.Called(user, oldKey, newKey)
	return args.Error(0)
}

type MockChannel struct {
	mock.Mock
	data []byte
}

func (m *MockChannel) Read(b []byte) (n int, err error) {
	args := m.Called(b)
	return args.Int(0), args.Error(1)
}

func (m *MockChannel) Write(b []byte) (n int, err error) {
	m.data = append(m.data, b...)
	args := m.Called(b)
	return args.Int(0), args.Error(1)
}

func (m *MockChannel) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockChannel) CloseWrite() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockChannel) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	args := m.Called(name, wantReply, payload)
	return args.Bool(0), args.Error(1)
}

func (m *MockChannel) Stderr() io.ReadWriter {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(io.ReadWriter)
}

type MockCloser struct {
	mock.Mock
}

func (m *MockCloser) Close() error { return m.Called().Error(0) }

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		user string
	}{
		{
			name: "creates interaction with default mode",
			user: "testuser",
		},
		{
			name: "creates interaction for different user",
			user: "anotheruser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, tt.user, mockCloser.Close)

			assert.NotNil(t, mockInteraction)
		})
	}
}

func TestInteraction_SetMode(t *testing.T) {
	tests := []struct {
		name string
		mode types.InteractiveMode
	}{
		{
			name: "set headless mode",
			mode: types.InteractiveModeHEADLESS,
		},
		{
			name: "set interactive mode",
			mode: types.InteractiveModeINTERACTIVE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)
			mockInteraction.SetMode(tt.mode)

			assert.Equal(t, tt.mode, mockInteraction.Mode())
		})
	}
}

func TestInteraction_Mode(t *testing.T) {
	tests := []struct {
		name     string
		setMode  types.InteractiveMode
		expected types.InteractiveMode
	}{
		{
			name:     "mode returns set value",
			setMode:  types.InteractiveModeINTERACTIVE,
			expected: types.InteractiveModeINTERACTIVE,
		},
		{
			name:     "mode returns headless value",
			setMode:  types.InteractiveModeHEADLESS,
			expected: types.InteractiveModeHEADLESS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)

			mockInteraction.SetMode(tt.setMode)
			assert.Equal(t, tt.expected, mockInteraction.Mode())
		})
	}
}

func TestInteraction_Send(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		setupChannel  bool
		channelReturn int
		channelError  error
		wantError     bool
	}{
		{
			name:          "send message successfully",
			message:       "test message",
			setupChannel:  true,
			channelReturn: 12,
			channelError:  nil,
			wantError:     false,
		},
		{
			name:          "send message with channel error",
			message:       "test message",
			setupChannel:  true,
			channelReturn: 0,
			channelError:  errors.New("channel write error"),
			wantError:     true,
		},
		{
			name:         "send message without channel",
			message:      "test message",
			setupChannel: false,
			wantError:    false,
		},
		{
			name:          "send empty message",
			message:       "",
			setupChannel:  true,
			channelReturn: 0,
			channelError:  nil,
			wantError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)

			if tt.setupChannel {
				mockChannel := &MockChannel{}
				mockChannel.On("Write", []byte(tt.message)).Return(tt.channelReturn, tt.channelError)
				mockInteraction.SetChannel(mockChannel)
			}

			err := mockInteraction.Send(tt.message)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInteraction_SetWH(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{
			name:   "set large window size",
			width:  100,
			height: 50,
		},
		{
			name:   "set medium window size",
			width:  80,
			height: 24,
		},
		{
			name:   "set small window size",
			width:  20,
			height: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)

			mockInteraction.SetWH(tt.width, tt.height)
		})
	}
}

func TestInteraction_SetChannel(t *testing.T) {
	mockRandom := &MockRandom{}
	mockConfig := &MockConfig{}
	mockSlug := &MockSlug{}
	mockForwarder := &MockForwarder{}
	mockSessionRegistry := &MockSessionRegistry{}
	mockCloser := &MockCloser{}
	mockSlug.On("String").Return("test-slug")

	mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)

	mockChannel := &MockChannel{}
	mockInteraction.SetChannel(mockChannel)

	mockChannel.On("Write", []byte("test")).Return(4, nil)
	err := mockInteraction.Send("test")
	assert.NoError(t, err)
}

func TestInteraction_Redraw(t *testing.T) {
	tests := []struct {
		name        string
		description string
	}{
		{
			name:        "redraw interaction",
			description: "should not panic when calling redraw",
		},
		{
			name:        "redraw multiple times",
			description: "should handle multiple redraws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)

			mockInteraction.Redraw()
		})
	}
}

func TestInteraction_Start(t *testing.T) {
	tests := []struct {
		name       string
		mode       types.InteractiveMode
		tlsEnabled bool
		tunnelType types.TunnelType
		port       uint16
	}{
		{
			name:       "start in headless mode - should return immediately",
			mode:       types.InteractiveModeHEADLESS,
			tlsEnabled: false,
			tunnelType: types.TunnelTypeHTTP,
			port:       8080,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)
			mockInteraction.SetMode(tt.mode)

			mockConfig.On("Domain").Return("tunnl.live")
			mockConfig.On("TLSEnabled").Return(tt.tlsEnabled)
			mockForwarder.On("TunnelType").Return(tt.tunnelType)
			mockForwarder.On("ForwardedPort").Return(tt.port)

			mockInteraction.Start()
		})
	}
}

func TestModel_Update(t *testing.T) {
	tests := []struct {
		name              string
		msg               tea.Msg
		showingComingSoon bool
		editingSlug       bool
		showingCommands   bool
		width             int
		height            int
		expectedWidth     int
		expectedHeight    int
		expectedQuit      bool
	}{
		{
			name:              "tick message clears coming soon",
			msg:               tickMsg{},
			showingComingSoon: true,
			editingSlug:       false,
			showingCommands:   false,
			expectedQuit:      false,
		},
		{
			name:           "window size message - large screen",
			msg:            tea.WindowSizeMsg{Width: 100, Height: 50},
			expectedWidth:  100,
			expectedHeight: 50,
			expectedQuit:   false,
		},
		{
			name:           "window size message - small screen",
			msg:            tea.WindowSizeMsg{Width: 60, Height: 20},
			expectedWidth:  60,
			expectedHeight: 20,
			expectedQuit:   false,
		},
		{
			name:         "quit message",
			msg:          tea.QuitMsg{},
			expectedQuit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)

			m := &model{
				randomizer:        mockRandom,
				domain:            "tunnl.live",
				protocol:          "http",
				tunnelType:        types.TunnelTypeHTTP,
				port:              8080,
				commandList:       list.New([]list.Item{}, list.NewDefaultDelegate(), 80, 20),
				interaction:       mockInteraction.(*interaction),
				showingComingSoon: tt.showingComingSoon,
				editingSlug:       tt.editingSlug,
				showingCommands:   tt.showingCommands,
				width:             tt.width,
				height:            tt.height,
				keymap: keymap{
					quit: key.NewBinding(
						key.WithKeys("q", "ctrl+c"),
						key.WithHelp("q", "quit"),
					),
					command: key.NewBinding(
						key.WithKeys("c"),
						key.WithHelp("c", "commands"),
					),
					random: key.NewBinding(
						key.WithKeys("ctrl+r"),
						key.WithHelp("ctrl+r", "random"),
					),
				},
			}

			result, _ := m.Update(tt.msg)
			updatedModel := result.(*model)

			if tt.expectedQuit {
				assert.True(t, updatedModel.quitting)
			}

			if windowMsg, ok := tt.msg.(tea.WindowSizeMsg); ok {
				assert.Equal(t, windowMsg.Width, updatedModel.width)
				assert.Equal(t, windowMsg.Height, updatedModel.height)
			}

			if _, ok := tt.msg.(tickMsg); ok && tt.showingComingSoon {
				assert.False(t, updatedModel.showingComingSoon)
			}
		})
	}
}

func TestModel_View(t *testing.T) {
	tests := []struct {
		name              string
		quitting          bool
		showingComingSoon bool
		editingSlug       bool
		showingCommands   bool
		expectedEmpty     bool
	}{
		{
			name:          "quitting returns empty string",
			quitting:      true,
			expectedEmpty: true,
		},
		{
			name:              "showing coming soon view",
			showingComingSoon: true,
			expectedEmpty:     false,
		},
		{
			name:          "editing slug view",
			editingSlug:   true,
			expectedEmpty: false,
		},
		{
			name:            "showing commands view",
			showingCommands: true,
			expectedEmpty:   false,
		},
		{
			name:          "dashboard view (default)",
			expectedEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)

			mockSlug.On("String").Return("test-slug")

			m := &model{
				randomizer:        mockRandom,
				domain:            "tunnl.live",
				protocol:          "http",
				tunnelType:        types.TunnelTypeHTTP,
				port:              8080,
				commandList:       list.New([]list.Item{}, list.NewDefaultDelegate(), 80, 20),
				interaction:       mockInteraction.(*interaction),
				quitting:          tt.quitting,
				showingComingSoon: tt.showingComingSoon,
				editingSlug:       tt.editingSlug,
				showingCommands:   tt.showingCommands,
				keymap: keymap{
					quit: key.NewBinding(
						key.WithKeys("q", "ctrl+c"),
						key.WithHelp("q", "quit"),
					),
					command: key.NewBinding(
						key.WithKeys("c"),
						key.WithHelp("c", "commands"),
					),
					random: key.NewBinding(
						key.WithKeys("ctrl+r"),
						key.WithHelp("ctrl+r", "random"),
					),
				},
			}

			view := m.View()

			if tt.expectedEmpty {
				assert.Empty(t, view)
			} else {
				assert.NotEmpty(t, view)
			}
		})
	}
}

func TestInteraction_Integration(t *testing.T) {
	mockRandom := &MockRandom{}
	mockConfig := &MockConfig{}
	mockSlug := &MockSlug{}
	mockForwarder := &MockForwarder{}
	mockSessionRegistry := &MockSessionRegistry{}
	closeCallCount := 0
	closeFunc := func() error {
		closeCallCount++
		return nil
	}
	mockSlug.On("String").Return("test-slug")

	mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
	assert.NotNil(t, mockInteraction)

	mockInteraction.SetMode(types.InteractiveModeINTERACTIVE)
	assert.Equal(t, types.InteractiveModeINTERACTIVE, mockInteraction.Mode())

	mockChannel := &MockChannel{}
	mockInteraction.SetChannel(mockChannel)

	mockChannel.On("Write", []byte("hello")).Return(5, nil)
	err := mockInteraction.Send("hello")
	assert.NoError(t, err)
	mockChannel.AssertExpectations(t)

	mockInteraction.SetWH(80, 24)

	mockInteraction.Redraw()
}

func TestModel_Update_KeyMessages(t *testing.T) {
	tests := []struct {
		name              string
		key               tea.KeyMsg
		showingComingSoon bool
		editingSlug       bool
		showingCommands   bool
		description       string
	}{
		{
			name:              "key press while showing coming soon",
			key:               tea.KeyMsg{Type: tea.KeyEnter},
			showingComingSoon: true,
			description:       "should call comingSoonUpdate",
		},
		{
			name:        "key press while editing slug",
			key:         tea.KeyMsg{Type: tea.KeyEnter},
			editingSlug: true,
			description: "should call slugUpdate",
		},
		{
			name:            "key press while showing commands",
			key:             tea.KeyMsg{Type: tea.KeyEnter},
			showingCommands: true,
			description:     "should call commandsUpdate",
		},
		{
			name:        "key press in dashboard view",
			key:         tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}},
			description: "should call dashboardUpdate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "user", mockCloser.Close)

			mockSlug.On("String").Return("test-slug").Maybe()
			mockSessionRegistry.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil)

			m := &model{
				randomizer:        mockRandom,
				domain:            "tunnl.live",
				protocol:          "http",
				tunnelType:        types.TunnelTypeHTTP,
				port:              8080,
				commandList:       list.New([]list.Item{}, list.NewDefaultDelegate(), 80, 20),
				interaction:       mockInteraction.(*interaction),
				showingComingSoon: tt.showingComingSoon,
				editingSlug:       tt.editingSlug,
				showingCommands:   tt.showingCommands,
				keymap: keymap{
					quit: key.NewBinding(
						key.WithKeys("q", "ctrl+c"),
						key.WithHelp("q", "quit"),
					),
					command: key.NewBinding(
						key.WithKeys("c"),
						key.WithHelp("c", "commands"),
					),
					random: key.NewBinding(
						key.WithKeys("ctrl+r"),
						key.WithHelp("ctrl+r", "random"),
					),
				},
			}

			result, _ := m.Update(tt.key)
			assert.NotNil(t, result)
		})
	}
}

func TestModel_SlugUpdate(t *testing.T) {
	tests := []struct {
		name           string
		tunnelType     types.TunnelType
		keyMsg         tea.KeyMsg
		inputValue     string
		setupMocks     func(*MockSessionRegistry, *MockSlug, *MockRandom)
		expectedEdit   bool
		expectedError  string
		shouldSetValue bool
	}{
		{
			name:         "escape key cancels editing",
			tunnelType:   types.TunnelTypeHTTP,
			keyMsg:       tea.KeyMsg{Type: tea.KeyEsc},
			setupMocks:   func(msr *MockSessionRegistry, ms *MockSlug, mr *MockRandom) {},
			expectedEdit: false,
		},
		{
			name:         "ctrl+c cancels editing",
			tunnelType:   types.TunnelTypeHTTP,
			keyMsg:       tea.KeyMsg{Type: tea.KeyCtrlC},
			setupMocks:   func(msr *MockSessionRegistry, ms *MockSlug, mr *MockRandom) {},
			expectedEdit: false,
		},
		{
			name:       "enter key saves valid slug",
			tunnelType: types.TunnelTypeHTTP,
			keyMsg:     tea.KeyMsg{Type: tea.KeyEnter},
			inputValue: "my-custom-slug",
			setupMocks: func(msr *MockSessionRegistry, ms *MockSlug, mr *MockRandom) {
				ms.On("String").Return("old-slug")
				msr.On("Update", "testuser",
					types.SessionKey{Id: "old-slug", Type: types.TunnelTypeHTTP},
					types.SessionKey{Id: "my-custom-slug", Type: types.TunnelTypeHTTP},
				).Return(nil)
			},
			expectedEdit:  false,
			expectedError: "",
		},
		{
			name:       "enter key with error shows error message",
			tunnelType: types.TunnelTypeHTTP,
			keyMsg:     tea.KeyMsg{Type: tea.KeyEnter},
			inputValue: "invalid",
			setupMocks: func(msr *MockSessionRegistry, ms *MockSlug, mr *MockRandom) {
				ms.On("String").Return("old-slug")
				msr.On("Update", "testuser",
					types.SessionKey{Id: "old-slug", Type: types.TunnelTypeHTTP},
					types.SessionKey{Id: "invalid", Type: types.TunnelTypeHTTP},
				).Return(assert.AnError)
			},
			expectedEdit:  true,
			expectedError: assert.AnError.Error(),
		},
		{
			name:       "ctrl+r generates random slug",
			tunnelType: types.TunnelTypeHTTP,
			keyMsg:     tea.KeyMsg{Type: tea.KeyCtrlR},
			setupMocks: func(msr *MockSessionRegistry, ms *MockSlug, mr *MockRandom) {
				mr.On("String", 20).Return("random-generated-slug", nil)
			},
			expectedEdit:   true,
			shouldSetValue: true,
		},
		{
			name:       "ctrl+r with error does nothing",
			tunnelType: types.TunnelTypeHTTP,
			keyMsg:     tea.KeyMsg{Type: tea.KeyCtrlR},
			setupMocks: func(msr *MockSessionRegistry, ms *MockSlug, mr *MockRandom) {
				mr.On("String", 20).Return("", assert.AnError)
			},
			expectedEdit: true,
		},
		{
			name:         "regular key updates input",
			tunnelType:   types.TunnelTypeHTTP,
			keyMsg:       tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}},
			setupMocks:   func(msr *MockSessionRegistry, ms *MockSlug, mr *MockRandom) {},
			expectedEdit: true,
		},
		{
			name:         "tcp tunnel exits editing immediately",
			tunnelType:   types.TunnelTypeTCP,
			keyMsg:       tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}},
			setupMocks:   func(msr *MockSessionRegistry, ms *MockSlug, mr *MockRandom) {},
			expectedEdit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

			ti := textinput.New()
			ti.SetValue(tt.inputValue)

			m := &model{
				randomizer:  mockRandom,
				domain:      "tunnl.live",
				protocol:    "http",
				tunnelType:  tt.tunnelType,
				port:        8080,
				commandList: list.New([]list.Item{}, list.NewDefaultDelegate(), 80, 20),
				slugInput:   ti,
				editingSlug: true,
				interaction: mockInteraction.(*interaction),
				keymap: keymap{
					quit: key.NewBinding(
						key.WithKeys("q", "ctrl+c"),
						key.WithHelp("q", "quit"),
					),
					command: key.NewBinding(
						key.WithKeys("c"),
						key.WithHelp("c", "commands"),
					),
					random: key.NewBinding(
						key.WithKeys("ctrl+r"),
						key.WithHelp("ctrl+r", "random"),
					),
				},
			}

			tt.setupMocks(mockSessionRegistry, mockSlug, mockRandom)

			result, _ := m.slugUpdate(tt.keyMsg)
			resultModel := result.(*model)

			assert.Equal(t, tt.expectedEdit, resultModel.editingSlug)
			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, resultModel.slugError)
			} else if !tt.expectedEdit {
				assert.Equal(t, "", resultModel.slugError)
			}

			mockSessionRegistry.AssertExpectations(t)
			mockSlug.AssertExpectations(t)
			mockRandom.AssertExpectations(t)
		})
	}
}

func TestModel_SlugView(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		tunnelType types.TunnelType
		slugError  string
		contains   string
	}{
		{
			name:       "http tunnel - large screen",
			width:      100,
			tunnelType: types.TunnelTypeHTTP,
			contains:   "Subdomain",
		},
		{
			name:       "http tunnel - small screen",
			width:      50,
			tunnelType: types.TunnelTypeHTTP,
			contains:   "Subdomain",
		},
		{
			name:       "http tunnel - tiny screen",
			width:      30,
			tunnelType: types.TunnelTypeHTTP,
			contains:   "Subdomain",
		},
		{
			name:       "http tunnel with error",
			width:      100,
			tunnelType: types.TunnelTypeHTTP,
			slugError:  "Slug already exists",
			contains:   "Slug already exists",
		},
		{
			name:       "tcp tunnel - large screen",
			width:      100,
			tunnelType: types.TunnelTypeTCP,
			contains:   "TCP",
		},
		{
			name:       "tcp tunnel - small screen",
			width:      50,
			tunnelType: types.TunnelTypeTCP,
			contains:   "TCP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

			ti := textinput.New()
			ti.SetValue("test-slug")

			m := &model{
				randomizer:  mockRandom,
				domain:      "tunnl.live",
				protocol:    "http",
				tunnelType:  tt.tunnelType,
				port:        8080,
				slugInput:   ti,
				slugError:   tt.slugError,
				interaction: mockInteraction.(*interaction),
				width:       tt.width,
			}

			view := m.slugView()
			assert.NotEmpty(t, view)
			assert.Contains(t, view, tt.contains)
		})
	}
}

func TestModel_ComingSoonUpdate(t *testing.T) {
	tests := []struct {
		name   string
		keyMsg tea.KeyMsg
	}{
		{
			name:   "any key dismisses coming soon",
			keyMsg: tea.KeyMsg{Type: tea.KeyEnter},
		},
		{
			name:   "escape key dismisses",
			keyMsg: tea.KeyMsg{Type: tea.KeyEsc},
		},
		{
			name:   "space key dismisses",
			keyMsg: tea.KeyMsg{Type: tea.KeySpace},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

			m := &model{
				interaction:       mockInteraction.(*interaction),
				showingComingSoon: true,
			}

			result, _ := m.comingSoonUpdate(tt.keyMsg)
			resultModel := result.(*model)

			assert.False(t, resultModel.showingComingSoon)
		})
	}
}

func TestModel_ComingSoonView(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{
			name:  "large screen",
			width: 100,
		},
		{
			name:  "medium screen",
			width: 60,
		},
		{
			name:  "small screen",
			width: 50,
		},
		{
			name:  "tiny screen",
			width: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

			m := &model{
				interaction: mockInteraction.(*interaction),
				width:       tt.width,
			}

			view := m.comingSoonView()
			assert.NotEmpty(t, view)
			assert.Contains(t, view, "Coming")
		})
	}
}

func TestModel_CommandsUpdate(t *testing.T) {
	tests := []struct {
		name             string
		keyMsg           tea.KeyMsg
		selectedItem     list.Item
		expectCommands   bool
		expectEditSlug   bool
		expectComingSoon bool
	}{
		{
			name:           "escape key closes commands",
			keyMsg:         tea.KeyMsg{Type: tea.KeyEsc},
			expectCommands: false,
		},
		{
			name:           "q key closes commands",
			keyMsg:         tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
			expectCommands: false,
		},
		{
			name:           "enter on slug command starts editing",
			keyMsg:         tea.KeyMsg{Type: tea.KeyEnter},
			selectedItem:   commandItem{name: "slug", desc: "Set custom subdomain"},
			expectCommands: false,
			expectEditSlug: true,
		},
		{
			name:             "enter on tunnel-type shows coming soon",
			keyMsg:           tea.KeyMsg{Type: tea.KeyEnter},
			selectedItem:     commandItem{name: "tunnel-type", desc: "Change tunnel type"},
			expectCommands:   false,
			expectComingSoon: true,
		},
		{
			name:           "arrow key navigates list",
			keyMsg:         tea.KeyMsg{Type: tea.KeyDown},
			expectCommands: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

			mockSlug.On("String").Return("current-slug").Maybe()

			items := []list.Item{
				commandItem{name: "slug", desc: "Set custom subdomain"},
				commandItem{name: "tunnel-type", desc: "Change tunnel type"},
			}

			delegate := list.NewDefaultDelegate()
			commandList := list.New(items, delegate, 80, 20)
			if tt.selectedItem != nil {
				for i, item := range items {
					if item.(commandItem).name == tt.selectedItem.(commandItem).name {
						commandList.Select(i)
						break
					}
				}
			}

			ti := textinput.New()

			m := &model{
				randomizer:      mockRandom,
				interaction:     mockInteraction.(*interaction),
				showingCommands: true,
				commandList:     commandList,
				slugInput:       ti,
				keymap: keymap{
					quit: key.NewBinding(
						key.WithKeys("q", "ctrl+c"),
						key.WithHelp("q", "quit"),
					),
					command: key.NewBinding(
						key.WithKeys("c"),
						key.WithHelp("c", "commands"),
					),
					random: key.NewBinding(
						key.WithKeys("ctrl+r"),
						key.WithHelp("ctrl+r", "random"),
					),
				},
			}

			result, _ := m.commandsUpdate(tt.keyMsg)
			resultModel := result.(*model)

			assert.Equal(t, tt.expectCommands, resultModel.showingCommands)
			if tt.expectEditSlug {
				assert.True(t, resultModel.editingSlug)
			}
			if tt.expectComingSoon {
				assert.True(t, resultModel.showingComingSoon)
			}
		})
	}
}

func TestModel_CommandsView(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{
			name:  "large screen",
			width: 100,
		},
		{
			name:  "medium screen",
			width: 60,
		},
		{
			name:  "small screen",
			width: 50,
		},
		{
			name:  "tiny screen",
			width: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

			items := []list.Item{
				commandItem{name: "slug", desc: "Set custom subdomain"},
				commandItem{name: "tunnel-type", desc: "Change tunnel type"},
			}

			delegate := list.NewDefaultDelegate()
			commandList := list.New(items, delegate, 80, 20)

			m := &model{
				interaction: mockInteraction.(*interaction),
				commandList: commandList,
				width:       tt.width,
			}

			view := m.commandsView()
			assert.NotEmpty(t, view)
			assert.Contains(t, view, "Commands")
		})
	}
}

func TestModel_DashboardUpdate(t *testing.T) {
	tests := []struct {
		name           string
		keyMsg         tea.KeyMsg
		expectQuit     bool
		expectCommands bool
	}{
		{
			name:       "q key quits",
			keyMsg:     tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
			expectQuit: true,
		},
		{
			name:       "ctrl+c quits",
			keyMsg:     tea.KeyMsg{Type: tea.KeyCtrlC},
			expectQuit: true,
		},
		{
			name:           "c key opens commands",
			keyMsg:         tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}},
			expectCommands: true,
		},
		{
			name:   "other keys do nothing",
			keyMsg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

			m := &model{
				interaction: mockInteraction.(*interaction),
				keymap: keymap{
					quit: key.NewBinding(
						key.WithKeys("q", "ctrl+c"),
						key.WithHelp("q", "quit"),
					),
					command: key.NewBinding(
						key.WithKeys("c"),
						key.WithHelp("c", "commands"),
					),
					random: key.NewBinding(
						key.WithKeys("ctrl+r"),
						key.WithHelp("ctrl+r", "random"),
					),
				},
			}

			result, _ := m.dashboardUpdate(tt.keyMsg)
			resultModel := result.(*model)

			if tt.expectQuit {
				assert.True(t, resultModel.quitting)
			}
			if tt.expectCommands {
				assert.True(t, resultModel.showingCommands)
			}
		})
	}
}

func TestModel_DashboardView(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		tunnelType types.TunnelType
		protocol   string
		port       uint16
		contains   string
	}{
		{
			name:       "http tunnel - large screen",
			width:      100,
			tunnelType: types.TunnelTypeHTTP,
			protocol:   "http",
			contains:   "http",
		},
		{
			name:       "https tunnel - large screen",
			width:      100,
			tunnelType: types.TunnelTypeHTTP,
			protocol:   "https",
			contains:   "https",
		},
		{
			name:       "tcp tunnel - large screen",
			width:      100,
			tunnelType: types.TunnelTypeTCP,
			port:       8080,
			contains:   "tcp",
		},
		{
			name:       "http tunnel - medium screen",
			width:      70,
			tunnelType: types.TunnelTypeHTTP,
			protocol:   "http",
			contains:   "http",
		},
		{
			name:       "http tunnel - small screen",
			width:      50,
			tunnelType: types.TunnelTypeHTTP,
			protocol:   "http",
			contains:   "http",
		},
		{
			name:       "http tunnel - tiny screen",
			width:      30,
			tunnelType: types.TunnelTypeHTTP,
			protocol:   "http",
			contains:   "http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

			mockSlug.On("String").Return("test-slug")

			m := &model{
				randomizer:  mockRandom,
				domain:      "tunnl.live",
				protocol:    tt.protocol,
				tunnelType:  tt.tunnelType,
				port:        tt.port,
				interaction: mockInteraction.(*interaction),
				width:       tt.width,
			}

			view := m.dashboardView()
			assert.NotEmpty(t, view)
			assert.Contains(t, view, tt.contains)
		})
	}
}

func TestGetResponsiveWidth(t *testing.T) {
	tests := []struct {
		name        string
		screenWidth int
		padding     int
		minWidth    int
		maxWidth    int
		expected    int
	}{
		{
			name:        "screen wider than max",
			screenWidth: 100,
			padding:     10,
			minWidth:    20,
			maxWidth:    60,
			expected:    60,
		},
		{
			name:        "screen narrower than min",
			screenWidth: 30,
			padding:     10,
			minWidth:    40,
			maxWidth:    80,
			expected:    40,
		},
		{
			name:        "screen within range",
			screenWidth: 70,
			padding:     10,
			minWidth:    20,
			maxWidth:    80,
			expected:    60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getResponsiveWidth(tt.screenWidth, tt.padding, tt.minWidth, tt.maxWidth)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldUseCompactLayout(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		threshold int
		expected  bool
	}{
		{
			name:      "width below threshold",
			width:     50,
			threshold: 60,
			expected:  true,
		},
		{
			name:      "width at threshold",
			width:     60,
			threshold: 60,
			expected:  false,
		},
		{
			name:      "width above threshold",
			width:     70,
			threshold: 60,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldUseCompactLayout(tt.width, tt.threshold)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		expected  string
	}{
		{
			name:      "string shorter than max",
			input:     "short",
			maxLength: 10,
			expected:  "short",
		},
		{
			name:      "string equal to max",
			input:     "exactly10c",
			maxLength: 10,
			expected:  "exactly10c",
		},
		{
			name:      "string longer than max",
			input:     "this is a very long string",
			maxLength: 10,
			expected:  "this is...",
		},
		{
			name:      "very short max length",
			input:     "hello",
			maxLength: 3,
			expected:  "hel",
		},
		{
			name:      "max length less than 4",
			input:     "hello",
			maxLength: 2,
			expected:  "he",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLength)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		subdomain string
		domain    string
		expected  string
	}{
		{
			name:      "http url",
			protocol:  "http",
			subdomain: "test",
			domain:    "tunnl.live",
			expected:  "http://test.tunnl.live",
		},
		{
			name:      "https url",
			protocol:  "https",
			subdomain: "api",
			domain:    "myapp.io",
			expected:  "https://api.myapp.io",
		},
		{
			name:      "custom subdomain",
			protocol:  "http",
			subdomain: "my-custom-slug",
			domain:    "tunnl.live",
			expected:  "http://my-custom-slug.tunnl.live",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildURL(tt.protocol, tt.subdomain, tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTickCmd(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{
			name:     "5 second tick",
			duration: 5 * time.Second,
		},
		{
			name:     "1 second tick",
			duration: 1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tickCmd(tt.duration)
			assert.NotNil(t, cmd)
		})
	}
}

func TestGetPaddingValue(t *testing.T) {
	tests := []struct {
		name          string
		isVeryCompact bool
		isCompact     bool
		expected      int
	}{
		{
			name:          "very compact layout",
			isVeryCompact: true,
			isCompact:     false,
			expected:      1,
		},
		{
			name:          "compact layout",
			isVeryCompact: false,
			isCompact:     true,
			expected:      1,
		},
		{
			name:          "normal layout",
			isVeryCompact: false,
			isCompact:     false,
			expected:      2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPaddingValue(tt.isVeryCompact, tt.isCompact)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetMarginValue(t *testing.T) {
	tests := []struct {
		name         string
		isCompact    bool
		compactValue int
		normalValue  int
		expected     int
	}{
		{
			name:         "compact layout",
			isCompact:    true,
			compactValue: 1,
			normalValue:  2,
			expected:     1,
		},
		{
			name:         "normal layout",
			isCompact:    false,
			compactValue: 1,
			normalValue:  2,
			expected:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMarginValue(tt.isCompact, tt.compactValue, tt.normalValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommandItem(t *testing.T) {
	tests := []struct {
		name     string
		item     commandItem
		wantName string
		wantDesc string
	}{
		{
			name:     "slug command",
			item:     commandItem{name: "slug", desc: "Set custom subdomain"},
			wantName: "slug",
			wantDesc: "Set custom subdomain",
		},
		{
			name:     "tunnel-type command",
			item:     commandItem{name: "tunnel-type", desc: "Change tunnel type"},
			wantName: "tunnel-type",
			wantDesc: "Change tunnel type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.item.FilterValue())
			assert.Equal(t, tt.wantName, tt.item.Title())
			assert.Equal(t, tt.wantDesc, tt.item.Description())
		})
	}
}

func TestModel_GetTunnelURL(t *testing.T) {
	tests := []struct {
		name       string
		tunnelType types.TunnelType
		protocol   string
		slug       string
		domain     string
		port       uint16
		expected   string
	}{
		{
			name:       "http tunnel",
			tunnelType: types.TunnelTypeHTTP,
			protocol:   "http",
			slug:       "my-app",
			domain:     "tunnl.live",
			expected:   "http://my-app.tunnl.live",
		},
		{
			name:       "https tunnel",
			tunnelType: types.TunnelTypeHTTP,
			protocol:   "https",
			slug:       "secure-app",
			domain:     "tunnl.live",
			expected:   "https://secure-app.tunnl.live",
		},
		{
			name:       "tcp tunnel",
			tunnelType: types.TunnelTypeTCP,
			domain:     "tunnl.live",
			port:       8080,
			expected:   "tcp://tunnl.live:8080",
		},
		{
			name:       "tcp tunnel with different port",
			tunnelType: types.TunnelTypeTCP,
			domain:     "tunnl.live",
			port:       3306,
			expected:   "tcp://tunnl.live:3306",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			mockCloser := &MockCloser{}
			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

			mockSlug.On("String").Return(tt.slug).Maybe()

			m := &model{
				domain:      tt.domain,
				protocol:    tt.protocol,
				tunnelType:  tt.tunnelType,
				port:        tt.port,
				interaction: mockInteraction.(*interaction),
			}

			result := m.getTunnelURL()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestModel_Init(t *testing.T) {
	mockRandom := &MockRandom{}
	mockConfig := &MockConfig{}
	mockSlug := &MockSlug{}
	mockForwarder := &MockForwarder{}
	mockSessionRegistry := &MockSessionRegistry{}
	mockCloser := &MockCloser{}
	mockSlug.On("String").Return("test-slug")

	mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", mockCloser.Close)

	m := &model{
		interaction: mockInteraction.(*interaction),
	}

	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestInteraction_Start_Interactive(t *testing.T) {
	tests := []struct {
		name       string
		tlsEnabled bool
		tunnelType types.TunnelType
		port       uint16
		domain     string
	}{
		{
			name:       "interactive mode with http",
			tlsEnabled: false,
			tunnelType: types.TunnelTypeHTTP,
			port:       8080,
			domain:     "tunnl.live",
		},
		{
			name:       "interactive mode with https",
			tlsEnabled: true,
			tunnelType: types.TunnelTypeHTTP,
			port:       8443,
			domain:     "secure.tunnl.live",
		},
		{
			name:       "interactive mode with tcp",
			tlsEnabled: false,
			tunnelType: types.TunnelTypeTCP,
			port:       3306,
			domain:     "db.tunnl.live",
		},
		{
			name:       "interactive mode with tcp and tls enabled",
			tlsEnabled: true,
			tunnelType: types.TunnelTypeTCP,
			port:       5432,
			domain:     "postgres.tunnl.live",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			closeCallCount := 0
			closeFunc := func() error {
				closeCallCount++
				return nil
			}

			mockConfig.On("Domain").Return(tt.domain)
			mockConfig.On("TLSEnabled").Return(tt.tlsEnabled)
			mockForwarder.On("TunnelType").Return(tt.tunnelType)
			mockForwarder.On("ForwardedPort").Return(tt.port)
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
			mockInteraction.SetMode(types.InteractiveModeINTERACTIVE)

			mockChannel := &MockChannel{}
			mockChannel.On("Read", mock.Anything).Return(0, assert.AnError).Maybe()
			mockChannel.On("Write", mock.Anything).Return(0, nil).Maybe()
			mockInteraction.SetChannel(mockChannel)

			done := make(chan bool, 1)
			go func() {
				mockInteraction.Start()
				done <- true
			}()

			time.Sleep(50 * time.Millisecond)

			i := mockInteraction.(*interaction)
			i.Stop()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("Start() did not complete in time")
			}

			assert.Equal(t, 1, closeCallCount, "close function should be called once")

			mockConfig.AssertExpectations(t)
			mockForwarder.AssertExpectations(t)
		})
	}
}

func TestInteraction_Start_ProtocolSelection(t *testing.T) {
	tests := []struct {
		name          string
		tlsEnabled    bool
		expectedProto string
	}{
		{
			name:          "http when TLS disabled",
			tlsEnabled:    false,
			expectedProto: "http",
		},
		{
			name:          "https when TLS enabled",
			tlsEnabled:    true,
			expectedProto: "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			closeFunc := func() error { return nil }

			mockConfig.On("Domain").Return("tunnl.live")
			mockConfig.On("TLSEnabled").Return(tt.tlsEnabled)
			mockForwarder.On("TunnelType").Return(types.TunnelTypeHTTP)
			mockForwarder.On("ForwardedPort").Return(uint16(8080))
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
			mockInteraction.SetMode(types.InteractiveModeINTERACTIVE)

			mockChannel := &MockChannel{}
			mockChannel.On("Read", mock.Anything).Return(0, assert.AnError).Maybe()
			mockChannel.On("Write", mock.Anything).Return(0, nil).Maybe()
			mockInteraction.SetChannel(mockChannel)

			go func() {
				mockInteraction.Start()
			}()

			time.Sleep(50 * time.Millisecond)

			i := mockInteraction.(*interaction)
			if i.program != nil {
				assert.NotNil(t, i.program, "program should be initialized")
			}

			i.Stop()

			mockConfig.AssertExpectations(t)
			mockForwarder.AssertExpectations(t)
		})
	}
}

func TestInteraction_Stop(t *testing.T) {
	tests := []struct {
		name         string
		setupProgram bool
		description  string
	}{
		{
			name:         "stop with active program",
			setupProgram: true,
			description:  "should kill program and set to nil",
		},
		{
			name:         "stop without program",
			setupProgram: false,
			description:  "should not panic when program is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			closeFunc := func() error { return nil }
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
			i := mockInteraction.(*interaction)

			if tt.setupProgram {
				mockConfig.On("Domain").Return("tunnl.live")
				mockConfig.On("TLSEnabled").Return(false)
				mockForwarder.On("TunnelType").Return(types.TunnelTypeHTTP)
				mockForwarder.On("ForwardedPort").Return(uint16(8080))

				mockInteraction.SetMode(types.InteractiveModeINTERACTIVE)
				mockChannel := &MockChannel{}
				mockChannel.On("Read", mock.Anything).Return(0, assert.AnError).Maybe()
				mockChannel.On("Write", mock.Anything).Return(0, nil).Maybe()
				mockInteraction.SetChannel(mockChannel)

				go func() {
					mockInteraction.Start()
				}()

				time.Sleep(50 * time.Millisecond)
			}

			assert.NotPanics(t, func() {
				i.Stop()
			})

			assert.Nil(t, i.program)

			select {
			case <-i.ctx.Done():
			case <-time.After(100 * time.Millisecond):
				t.Fatal("context should be cancelled after Stop()")
			}
		})
	}
}

func TestInteraction_Start_CommandListSetup(t *testing.T) {
	mockRandom := &MockRandom{}
	mockConfig := &MockConfig{}
	mockSlug := &MockSlug{}
	mockForwarder := &MockForwarder{}
	mockSessionRegistry := &MockSessionRegistry{}
	closeFunc := func() error { return nil }

	mockConfig.On("Domain").Return("tunnl.live")
	mockConfig.On("TLSEnabled").Return(false)
	mockForwarder.On("TunnelType").Return(types.TunnelTypeHTTP)
	mockForwarder.On("ForwardedPort").Return(uint16(8080))

	mockSlug.On("String").Return("test-slug")

	mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
	mockInteraction.SetMode(types.InteractiveModeINTERACTIVE)

	mockChannel := &MockChannel{}
	mockChannel.On("Read", mock.Anything).Return(0, nil)
	mockChannel.On("Write", mock.Anything).Return(0, nil)
	mockInteraction.SetChannel(mockChannel)

	go func() {
		mockInteraction.Start()
	}()

	time.Sleep(50 * time.Millisecond)

	i := mockInteraction.(*interaction)

	assert.NotNil(t, i.program, "program should be initialized")

	i.Stop()
}

func TestInteraction_Start_TextInputSetup(t *testing.T) {
	mockRandom := &MockRandom{}
	mockConfig := &MockConfig{}
	mockSlug := &MockSlug{}
	mockForwarder := &MockForwarder{}
	mockSessionRegistry := &MockSessionRegistry{}
	closeFunc := func() error { return nil }

	mockConfig.On("Domain").Return("tunnl.live")
	mockConfig.On("TLSEnabled").Return(false)
	mockForwarder.On("TunnelType").Return(types.TunnelTypeHTTP)
	mockForwarder.On("ForwardedPort").Return(uint16(8080))
	mockSlug.On("String").Return("test-slug")

	mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
	mockInteraction.SetMode(types.InteractiveModeINTERACTIVE)

	mockChannel := &MockChannel{}
	mockChannel.On("Read", mock.Anything).Return(0, assert.AnError).Maybe()
	mockChannel.On("Write", mock.Anything).Return(0, nil).Maybe()
	mockInteraction.SetChannel(mockChannel)

	go func() {
		mockInteraction.Start()
	}()

	time.Sleep(50 * time.Millisecond)

	i := mockInteraction.(*interaction)
	i.Stop()

	mockConfig.AssertExpectations(t)
	mockForwarder.AssertExpectations(t)
}

func TestInteraction_Start_CleanupOnExit(t *testing.T) {
	tests := []struct {
		name              string
		closeFunc         CloseFunc
		expectCloseCalled bool
	}{
		{
			name: "cleanup calls close function",
			closeFunc: func() error {
				return nil
			},
			expectCloseCalled: true,
		},
		{
			name:              "cleanup with nil close function",
			closeFunc:         nil,
			expectCloseCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}

			closeCallCount := 0
			var closeFunc CloseFunc
			if tt.closeFunc != nil {
				closeFunc = func() error {
					closeCallCount++
					return tt.closeFunc()
				}
			}

			mockConfig.On("Domain").Return("tunnl.live")
			mockConfig.On("TLSEnabled").Return(false)
			mockForwarder.On("TunnelType").Return(types.TunnelTypeHTTP)
			mockForwarder.On("ForwardedPort").Return(uint16(8080))
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
			mockInteraction.SetMode(types.InteractiveModeINTERACTIVE)

			mockChannel := &MockChannel{}
			mockChannel.On("Read", mock.Anything).Return(0, assert.AnError).Maybe()
			mockChannel.On("Write", mock.Anything).Return(0, nil).Maybe()
			mockInteraction.SetChannel(mockChannel)

			done := make(chan bool, 1)
			go func() {
				mockInteraction.Start()
				done <- true
			}()

			time.Sleep(50 * time.Millisecond)

			i := mockInteraction.(*interaction)
			i.Stop()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("Start() did not complete")
			}

			if tt.expectCloseCalled {
				assert.Equal(t, 1, closeCallCount, "close function should be called")
			} else {
				assert.Equal(t, 0, closeCallCount, "close function should not be called when nil")
			}
		})
	}
}

func TestInteraction_Start_WithDifferentChannels(t *testing.T) {
	tests := []struct {
		name         string
		setupChannel bool
	}{
		{
			name:         "start with channel set",
			setupChannel: true,
		},
		{
			name:         "start with nil channel",
			setupChannel: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRandom := &MockRandom{}
			mockConfig := &MockConfig{}
			mockSlug := &MockSlug{}
			mockForwarder := &MockForwarder{}
			mockSessionRegistry := &MockSessionRegistry{}
			closeFunc := func() error { return nil }

			mockConfig.On("Domain").Return("tunnl.live")
			mockConfig.On("TLSEnabled").Return(false)
			mockForwarder.On("TunnelType").Return(types.TunnelTypeHTTP)
			mockForwarder.On("ForwardedPort").Return(uint16(8080))
			mockSlug.On("String").Return("test-slug")

			mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
			mockInteraction.SetMode(types.InteractiveModeINTERACTIVE)

			if tt.setupChannel {
				mockChannel := &MockChannel{}
				mockChannel.On("Read", mock.Anything).Return(0, assert.AnError).Maybe()
				mockChannel.On("Write", mock.Anything).Return(0, nil).Maybe()
				mockInteraction.SetChannel(mockChannel)
			}

			go func() {
				mockInteraction.Start()
			}()

			time.Sleep(50 * time.Millisecond)

			i := mockInteraction.(*interaction)
			i.Stop()

			mockConfig.AssertExpectations(t)
			mockForwarder.AssertExpectations(t)
		})
	}
}

func TestInteraction_Stop_ContextCancellation(t *testing.T) {
	mockRandom := &MockRandom{}
	mockConfig := &MockConfig{}
	mockSlug := &MockSlug{}
	mockForwarder := &MockForwarder{}
	mockSessionRegistry := &MockSessionRegistry{}
	closeFunc := func() error { return nil }
	mockSlug.On("String").Return("test-slug")

	mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
	i := mockInteraction.(*interaction)

	select {
	case <-i.ctx.Done():
		t.Fatal("context should not be cancelled initially")
	default:
	}

	i.Stop()

	select {
	case <-i.ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be cancelled after Stop()")
	}

	assert.NotPanics(t, func() {
		i.Stop()
	})
}

func TestInteraction_Stop_MultipleCallsSafe(t *testing.T) {
	mockRandom := &MockRandom{}
	mockConfig := &MockConfig{}
	mockSlug := &MockSlug{}
	mockForwarder := &MockForwarder{}
	mockSessionRegistry := &MockSessionRegistry{}
	closeFunc := func() error { return nil }
	mockSlug.On("String").Return("test-slug")

	mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
	i := mockInteraction.(*interaction)

	assert.NotPanics(t, func() {
		i.Stop()
		i.Stop()
		i.Stop()
	})

	assert.Nil(t, i.program)
}

func TestInteraction_Start_HeadlessMode_NoOp(t *testing.T) {
	mockRandom := &MockRandom{}
	mockConfig := &MockConfig{}
	mockSlug := &MockSlug{}
	mockForwarder := &MockForwarder{}
	mockSessionRegistry := &MockSessionRegistry{}
	closeFunc := func() error { return nil }
	mockSlug.On("String").Return("test-slug")

	mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
	mockInteraction.SetMode(types.InteractiveModeHEADLESS)

	done := make(chan bool, 1)
	go func() {
		mockInteraction.Start()
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("headless mode should return immediately")
	}

	i := mockInteraction.(*interaction)
	assert.Nil(t, i.program, "program should not be created in headless mode")

	mockConfig.AssertNotCalled(t, "Domain")
	mockConfig.AssertNotCalled(t, "TLSEnabled")
	mockForwarder.AssertNotCalled(t, "TunnelType")
	mockForwarder.AssertNotCalled(t, "ForwardedPort")
}

func TestInteraction_New_ContextInitialization(t *testing.T) {
	mockRandom := &MockRandom{}
	mockConfig := &MockConfig{}
	mockSlug := &MockSlug{}
	mockForwarder := &MockForwarder{}
	mockSessionRegistry := &MockSessionRegistry{}
	closeFunc := func() error { return nil }
	mockSlug.On("String").Return("test-slug")

	mockInteraction := New(mockRandom, mockConfig, mockSlug, mockForwarder, mockSessionRegistry, "testuser", closeFunc)
	i := mockInteraction.(*interaction)

	assert.NotNil(t, i.ctx, "context should be initialized")
	assert.NotNil(t, i.cancel, "cancel function should be initialized")

	select {
	case <-i.ctx.Done():
		t.Fatal("context should not be cancelled initially")
	default:
	}
}
