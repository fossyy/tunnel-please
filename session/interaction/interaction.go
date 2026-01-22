package interaction

import (
	"context"
	"log"
	"tunnel_pls/internal/config"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/crypto/ssh"
)

type Interaction interface {
	Mode() types.InteractiveMode
	SetChannel(channel ssh.Channel)
	SetMode(m types.InteractiveMode)
	SetWH(w, h int)
	Start()
	Redraw()
	Send(message string) error
}

type SessionRegistry interface {
	Update(user string, oldKey, newKey types.SessionKey) error
}

type Forwarder interface {
	Close() error
	TunnelType() types.TunnelType
	ForwardedPort() uint16
}

type CloseFunc func() error
type interaction struct {
	config          config.Config
	channel         ssh.Channel
	slug            slug.Slug
	forwarder       Forwarder
	closeFunc       CloseFunc
	user            string
	sessionRegistry SessionRegistry
	program         *tea.Program
	ctx             context.Context
	cancel          context.CancelFunc
	mode            types.InteractiveMode
}

func (i *interaction) SetMode(m types.InteractiveMode) {
	i.mode = m
}

func (i *interaction) Mode() types.InteractiveMode {
	return i.mode
}

func (i *interaction) Send(message string) error {
	if i.channel != nil {
		_, err := i.channel.Write([]byte(message))
		return err
	}
	return nil
}
func (i *interaction) SetWH(w, h int) {
	if i.program != nil {
		i.program.Send(tea.WindowSizeMsg{
			Width:  w,
			Height: h,
		})
	}
}

func New(config config.Config, slug slug.Slug, forwarder Forwarder, sessionRegistry SessionRegistry, user string, closeFunc CloseFunc) Interaction {
	ctx, cancel := context.WithCancel(context.Background())
	return &interaction{
		config:          config,
		channel:         nil,
		slug:            slug,
		forwarder:       forwarder,
		closeFunc:       closeFunc,
		user:            user,
		sessionRegistry: sessionRegistry,
		program:         nil,
		ctx:             ctx,
		cancel:          cancel,
	}
}

func (i *interaction) SetChannel(channel ssh.Channel) {
	i.channel = channel
}

func (i *interaction) Stop() {
	if i.cancel != nil {
		i.cancel()
	}
	if i.program != nil {
		i.program.Kill()
		i.program = nil
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg := msg.(type) {
	case tickMsg:
		m.showingComingSoon = false
		return m, tea.Batch(tea.ClearScreen, textinput.Blink)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.commandList.SetWidth(msg.Width)
		m.commandList.SetHeight(msg.Height - 4)

		if msg.Width < 80 {
			m.slugInput.Width = msg.Width - 10
		} else {
			m.slugInput.Width = 50
		}
		return m, nil

	case tea.QuitMsg:
		m.quitting = true
		return m, tea.Batch(tea.ClearScreen, textinput.Blink, tea.Quit)

	case tea.KeyMsg:
		if m.showingComingSoon {
			return m.comingSoonUpdate(msg)
		}

		if m.editingSlug {
			return m.slugUpdate(msg)
		}

		if m.showingCommands {
			return m.commandsUpdate(msg)
		}

		return m.dashboardUpdate(msg)
	}

	return m, nil
}

func (i *interaction) Redraw() {
	if i.program != nil {
		i.program.Send(tea.ClearScreen())
	}
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}

	if m.showingComingSoon {
		return m.comingSoonView()
	}

	if m.editingSlug {
		return m.slugView()
	}

	if m.showingCommands {
		return m.commandsView()
	}

	return m.dashboardView()
}

func (i *interaction) Start() {
	if i.mode == types.InteractiveModeHEADLESS {
		return
	}
	lipgloss.SetColorProfile(termenv.TrueColor)

	protocol := "http"
	if i.config.TLSEnabled() {
		protocol = "https"
	}

	tunnelType := i.forwarder.TunnelType()
	port := i.forwarder.ForwardedPort()

	items := []list.Item{
		commandItem{name: "slug", desc: "Set custom subdomain"},
		commandItem{name: "tunnel-type", desc: "Change tunnel type (Coming Soon)"},
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetHeight(2)

	commandList := list.New(items, delegate, 80, 20)
	commandList.Title = "Select a command"
	commandList.SetShowStatusBar(false)
	commandList.SetFilteringEnabled(false)
	commandList.SetShowHelp(false)

	ti := textinput.New()
	ti.Placeholder = "my-custom-slug"
	ti.CharLimit = 20
	ti.Width = 50

	m := &model{
		domain:      i.config.Domain(),
		protocol:    protocol,
		tunnelType:  tunnelType,
		port:        port,
		commandList: commandList,
		slugInput:   ti,
		interaction: i,
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
		help: help.New(),
	}

	i.program = tea.NewProgram(
		m,
		tea.WithInput(i.channel),
		tea.WithOutput(i.channel),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithoutSignals(),
		tea.WithoutSignalHandler(),
		tea.WithFPS(30),
	)

	_, err := i.program.Run()
	if err != nil {
		log.Printf("Cannot close tea: %s \n", err)
	}
	i.program.Kill()
	i.program = nil
	if i.closeFunc != nil {
		if err := i.closeFunc(); err != nil {
			log.Printf("Cannot close session: %s \n", err)
		}
	}
}
