package interaction

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"
	"tunnel_pls/utils"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/crypto/ssh"
)

type Lifecycle interface {
	Close() error
}

type Controller interface {
	SetChannel(channel ssh.Channel)
	SetLifecycle(lifecycle Lifecycle)
	SetSlugModificator(func(oldSlug, newSlug string) bool)
	Start()
}

type Forwarder interface {
	Close() error
	GetTunnelType() types.TunnelType
	GetForwardedPort() uint16
}

type Interaction struct {
	channel          ssh.Channel
	slugManager      slug.Manager
	forwarder        Forwarder
	lifecycle        Lifecycle
	updateClientSlug func(oldSlug, newSlug string) bool
	program          *tea.Program
	ctx              context.Context
	cancel           context.CancelFunc
}

type commandItem struct {
	name string
	desc string
}

type model struct {
	tunnelURL         string
	domain            string
	protocol          string
	tunnelType        types.TunnelType
	port              uint16
	keymap            keymap
	help              help.Model
	quitting          bool
	showingCommands   bool
	editingSlug       bool
	showingComingSoon bool
	commandList       list.Model
	slugInput         textinput.Model
	slugError         string
	interaction       *Interaction
}

type keymap struct {
	quit    key.Binding
	command key.Binding
	random  key.Binding
}

type tickMsg time.Time

func NewInteraction(slugManager slug.Manager, forwarder Forwarder) *Interaction {
	ctx, cancel := context.WithCancel(context.Background())
	return &Interaction{
		channel:          nil,
		slugManager:      slugManager,
		forwarder:        forwarder,
		lifecycle:        nil,
		updateClientSlug: nil,
		program:          nil,
		ctx:              ctx,
		cancel:           cancel,
	}
}

func (i *Interaction) SetLifecycle(lifecycle Lifecycle) {
	i.lifecycle = lifecycle
}

func (i *Interaction) SetChannel(channel ssh.Channel) {
	i.channel = channel
}

func (i *Interaction) SetSlugModificator(modificator func(oldSlug, newSlug string) bool) {
	i.updateClientSlug = modificator
}

func (i *Interaction) Stop() {
	if i.cancel != nil {
		i.cancel()
	}
	if i.program != nil {
		i.program.Kill()
		i.program = nil
	}
}

func (i commandItem) FilterValue() string { return i.name }
func (i commandItem) Title() string       { return i.name }
func (i commandItem) Description() string { return i.desc }

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tea.WindowSize())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tickMsg:
		m.showingComingSoon = false
		return m, tea.Batch(tea.ClearScreen, textinput.Blink)

	case tea.WindowSizeMsg:
		m.commandList.SetWidth(msg.Width)
		m.commandList.SetHeight(msg.Height - 4)
		return m, nil

	case tea.QuitMsg:
		m.quitting = true
		return m, tea.Batch(tea.ClearScreen, textinput.Blink, tea.Quit)

	case tea.KeyMsg:
		if m.showingComingSoon {
			m.showingComingSoon = false
			return m, tea.Batch(tea.ClearScreen, textinput.Blink)
		}

		if m.editingSlug {
			switch msg.String() {
			case "esc":
				m.editingSlug = false
				m.slugError = ""
				return m, tea.Batch(tea.ClearScreen, textinput.Blink)
			case "enter":
				inputValue := m.slugInput.Value()
				m.interaction.updateClientSlug(m.interaction.slugManager.Get(), inputValue)
				m.tunnelURL = buildURL(m.protocol, inputValue, m.domain)
				m.editingSlug = false
				m.slugError = ""
				return m, tea.Batch(tea.ClearScreen, textinput.Blink)
			case "ctrl+c":
				m.editingSlug = false
				m.slugError = ""
				return m, tea.Batch(tea.ClearScreen, textinput.Blink)
			default:
				if key.Matches(msg, m.keymap.random) {
					newSubdomain := generateRandomSubdomain()
					m.slugInput.SetValue(newSubdomain)
					m.slugError = ""
					m.slugInput, cmd = m.slugInput.Update(msg)
					return m, cmd
				}
				m.slugError = ""
				m.slugInput, cmd = m.slugInput.Update(msg)
				return m, cmd
			}
		}

		if m.showingCommands {
			switch {
			case key.Matches(msg, m.keymap.quit):
				m.showingCommands = false
				return m, tea.Batch(tea.ClearScreen, textinput.Blink)
			case msg.String() == "enter":
				selectedItem := m.commandList.SelectedItem()
				if selectedItem != nil {
					item := selectedItem.(commandItem)
					if item.name == "slug" {
						m.showingCommands = false
						m.editingSlug = true
						m.slugInput.SetValue(m.interaction.slugManager.Get())
						m.slugInput.Focus()
						return m, tea.Batch(tea.ClearScreen, textinput.Blink)
					} else if item.name == "tunnel-type" {
						m.showingCommands = false
						m.showingComingSoon = true
						return m, tea.Batch(tickCmd(5*time.Second), tea.ClearScreen, textinput.Blink)
					}
					m.showingCommands = false
					return m, nil
				}
			case msg.String() == "esc":
				m.showingCommands = false
				return m, tea.Batch(tea.ClearScreen, textinput.Blink)
			}
			m.commandList, cmd = m.commandList.Update(msg)
			return m, cmd
		}

		switch {
		case key.Matches(msg, m.keymap.quit):
			m.quitting = true
			return m, tea.Batch(tea.ClearScreen, textinput.Blink, tea.Quit)
		case key.Matches(msg, m.keymap.command):
			m.showingCommands = true
			return m, tea.Batch(tea.ClearScreen, textinput.Blink)
		}
	}

	return m, nil
}

func (m model) helpView() string {
	return "\n" + m.help.ShortHelpView([]key.Binding{
		m.keymap.command,
		m.keymap.quit,
	})
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	if m.showingComingSoon {
		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			PaddingTop(1).
			PaddingBottom(1)

		messageBoxStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#1A1A2E")).
			Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 3).
			MarginTop(2).
			MarginBottom(2).
			Align(lipgloss.Center)

		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true).
			MarginTop(1)

		var b strings.Builder
		b.WriteString("\n\n")
		b.WriteString(titleStyle.Render("⏳ Coming Soon"))
		b.WriteString("\n\n")
		b.WriteString(messageBoxStyle.Render("🚀 This feature is coming very soon!\n   Stay tuned for updates."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("This message will disappear in 5 seconds or press any key..."))

		return b.String()
	}

	if m.editingSlug {
		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			PaddingTop(1).
			PaddingBottom(1)

		instructionStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			MarginTop(1)

		inputBoxStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2).
			MarginTop(2).
			MarginBottom(2)

		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true).
			MarginTop(1)

		errorBoxStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Background(lipgloss.Color("#3D0000")).
			Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF0000")).
			Padding(0, 2).
			MarginTop(1).
			MarginBottom(1)

		rulesBoxStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 2).
			MarginTop(1).
			MarginBottom(1)

		var b strings.Builder
		b.WriteString(titleStyle.Render("🔧 Edit Subdomain"))
		b.WriteString("\n\n")

		if m.tunnelType != types.HTTP {
			warningBoxStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFA500")).
				Background(lipgloss.Color("#3D2000")).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FFA500")).
				Padding(1, 2).
				MarginTop(2).
				MarginBottom(2)

			b.WriteString(warningBoxStyle.Render("⚠️ TCP tunnels cannot have custom subdomains. Only HTTP/HTTPS tunnels support subdomain customization. "))
			b.WriteString("\n\n")
			b.WriteString(helpStyle.Render("Press Enter or Esc to go back"))
			return b.String()
		}

		rulesContent := "📋 Rules: \n\t• 3-20 chars \n\t• a-z, 0-9, - \n\t• No leading/trailing -"
		b.WriteString(rulesBoxStyle.Render(rulesContent))
		b.WriteString("\n")

		b.WriteString(instructionStyle.Render("Enter your custom subdomain:"))
		b.WriteString("\n")

		if m.slugError != "" {
			errorInputBoxStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FF0000")).
				Padding(1, 2).
				MarginTop(2).
				MarginBottom(1)
			b.WriteString(errorInputBoxStyle.Render(m.slugInput.View()))
			b.WriteString("\n")
			b.WriteString(errorBoxStyle.Render("❌ " + m.slugError))
			b.WriteString("\n")
		} else {
			b.WriteString(inputBoxStyle.Render(m.slugInput.View()))
			b.WriteString("\n")
		}

		previewURL := buildURL(m.protocol, m.slugInput.Value(), m.domain)
		previewStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Italic(true).
			Width(80)
		b.WriteString(previewStyle.Render(fmt.Sprintf("Preview: %s", previewURL)))
		b.WriteString("\n")

		b.WriteString(helpStyle.Render("Press Enter to save • CTRL+R for random • Esc to cancel"))

		return b.String()
	}

	if m.showingCommands {
		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			PaddingTop(1).
			PaddingBottom(1)

		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true).
			MarginTop(1)

		var b strings.Builder
		b.WriteString("\n")
		b.WriteString(titleStyle.Render("⚡ Commands"))
		b.WriteString("\n\n")
		b.WriteString(m.commandList.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("↑/↓ Navigate • Enter Select • Esc Cancel"))

		return b.String()
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		PaddingTop(1).
		PaddingBottom(1)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Italic(true)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		Underline(true)

	sectionTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		MarginTop(1).
		MarginBottom(1)

	forwardingStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#04B575")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#04B575")).
		Padding(0, 2).
		MarginTop(1).
		MarginBottom(1)

	var b strings.Builder

	b.WriteString(titleStyle.Render("🚇 Tunnel Pls"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Project by Bagas"))
	b.WriteString("\n")
	b.WriteString(urlStyle.Render("https://fossy.my.id"))
	b.WriteString("\n\n")

	b.WriteString(sectionTitleStyle.Render("Welcome to Tunnel!"))
	b.WriteString("\n")

	b.WriteString("\n")
	forwardingText := fmt.Sprintf("🌐 Forwarding your traffic to:\n   %s", m.tunnelURL)
	b.WriteString(forwardingStyle.Render(forwardingText))

	b.WriteString(m.helpView())

	return b.String()
}

func (i *Interaction) Start() {
	lipgloss.SetColorProfile(termenv.TrueColor)

	domain := utils.Getenv("DOMAIN", "localhost")
	protocol := "http"
	if utils.Getenv("TLS_ENABLED", "false") == "true" {
		protocol = "https"
	}

	tunnelType := i.forwarder.GetTunnelType()
	port := i.forwarder.GetForwardedPort()

	var tunnelURL string
	if tunnelType == types.HTTP {
		tunnelURL = buildURL(protocol, i.slugManager.Get(), domain)
	} else {
		tunnelURL = fmt.Sprintf("tcp://%s:%d", domain, port)
	}

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

	m := model{
		tunnelURL:   tunnelURL,
		domain:      domain,
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
	)

	_, err := i.program.Run()
	if err != nil {
		log.Printf("Cannot close tea: %s \n", err)
	}
	i.program.Kill()
	i.program = nil
	if err := m.interaction.lifecycle.Close(); err != nil {
		log.Printf("Cannot close session: %s \n", err)
	}
}

func buildURL(protocol, subdomain, domain string) string {
	return fmt.Sprintf("%s://%s.%s", protocol, subdomain, domain)
}

func generateRandomSubdomain() string {
	return utils.GenerateRandomString(20)
}
