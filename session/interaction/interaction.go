package interaction

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/random"
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

type Lifecycle interface {
	Close() error
	User() string
}

type SessionRegistry interface {
	Update(user string, oldKey, newKey types.SessionKey) error
}

type Interaction interface {
	Mode() types.Mode
	SetChannel(channel ssh.Channel)
	SetLifecycle(lifecycle Lifecycle)
	SetSessionRegistry(registry SessionRegistry)
	SetMode(m types.Mode)
	SetWH(w, h int)
	Start()
	Redraw()
	Send(message string) error
}

type Forwarder interface {
	Close() error
	TunnelType() types.TunnelType
	ForwardedPort() uint16
}

type interaction struct {
	channel         ssh.Channel
	slug            slug.Slug
	forwarder       Forwarder
	lifecycle       Lifecycle
	sessionRegistry SessionRegistry
	program         *tea.Program
	ctx             context.Context
	cancel          context.CancelFunc
	mode            types.Mode
}

func (i *interaction) SetMode(m types.Mode) {
	i.mode = m
}

func (i *interaction) Mode() types.Mode {
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

type commandItem struct {
	name string
	desc string
}

type model struct {
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
	interaction       *interaction
	width             int
	height            int
}

func (m *model) getTunnelURL() string {
	if m.tunnelType == types.HTTP {
		return buildURL(m.protocol, m.interaction.slug.String(), m.domain)
	}
	return fmt.Sprintf("tcp://%s:%d", m.domain, m.port)
}

type keymap struct {
	quit    key.Binding
	command key.Binding
	random  key.Binding
}

type tickMsg time.Time

func New(slug slug.Slug, forwarder Forwarder) Interaction {
	ctx, cancel := context.WithCancel(context.Background())
	return &interaction{
		channel:         nil,
		slug:            slug,
		forwarder:       forwarder,
		lifecycle:       nil,
		sessionRegistry: nil,
		program:         nil,
		ctx:             ctx,
		cancel:          cancel,
	}
}

func (i *interaction) SetSessionRegistry(registry SessionRegistry) {
	i.sessionRegistry = registry
}

func (i *interaction) SetLifecycle(lifecycle Lifecycle) {
	i.lifecycle = lifecycle
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

func getResponsiveWidth(screenWidth, padding, minWidth, maxWidth int) int {
	width := screenWidth - padding
	if width > maxWidth {
		width = maxWidth
	}
	if width < minWidth {
		width = minWidth
	}
	return width
}

func shouldUseCompactLayout(width int, threshold int) bool {
	return width < threshold
}

func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	if maxLength < 4 {
		return s[:maxLength]
	}
	return s[:maxLength-3] + "..."
}

func (i commandItem) FilterValue() string { return i.name }
func (i commandItem) Title() string       { return i.name }
func (i commandItem) Description() string { return i.desc }

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tea.WindowSize())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

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
			m.showingComingSoon = false
			return m, tea.Batch(tea.ClearScreen, textinput.Blink)
		}

		if m.editingSlug {
			if m.tunnelType != types.HTTP {
				m.editingSlug = false
				m.slugError = ""
				return m, tea.Batch(tea.ClearScreen, textinput.Blink)
			}
			switch msg.String() {
			case "esc":
				m.editingSlug = false
				m.slugError = ""
				return m, tea.Batch(tea.ClearScreen, textinput.Blink)
			case "enter":
				inputValue := m.slugInput.Value()
				if err := m.interaction.sessionRegistry.Update(m.interaction.lifecycle.User(), types.SessionKey{
					Id:   m.interaction.slug.String(),
					Type: types.HTTP,
				}, types.SessionKey{
					Id:   inputValue,
					Type: types.HTTP,
				}); err != nil {
					m.slugError = err.Error()
					return m, nil
				}
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
						m.slugInput.SetValue(m.interaction.slug.String())
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

func (i *interaction) Redraw() {
	if i.program != nil {
		i.program.Send(tea.ClearScreen())
	}
}

func (m *model) helpView() string {
	return "\n" + m.help.ShortHelpView([]key.Binding{
		m.keymap.command,
		m.keymap.quit,
	})
}

func (m *model) View() string {
	if m.quitting {
		return ""
	}

	if m.showingComingSoon {
		isCompact := shouldUseCompactLayout(m.width, 60)

		var boxPadding int
		var boxMargin int
		if isCompact {
			boxPadding = 1
			boxMargin = 1
		} else {
			boxPadding = 3
			boxMargin = 2
		}

		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			PaddingTop(1).
			PaddingBottom(1)

		messageBoxWidth := getResponsiveWidth(m.width, 10, 30, 60)
		messageBoxStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#1A1A2E")).
			Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, boxPadding).
			MarginTop(boxMargin).
			MarginBottom(boxMargin).
			Width(messageBoxWidth).
			Align(lipgloss.Center)

		helpStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true).
			MarginTop(1)

		var b strings.Builder
		b.WriteString("\n\n")

		var title string
		if shouldUseCompactLayout(m.width, 40) {
			title = "Coming Soon"
		} else {
			title = "⏳ Coming Soon"
		}
		b.WriteString(titleStyle.Render(title))
		b.WriteString("\n\n")

		var message string
		if shouldUseCompactLayout(m.width, 50) {
			message = "Coming soon!\nStay tuned."
		} else {
			message = "🚀 This feature is coming very soon!\n   Stay tuned for updates."
		}
		b.WriteString(messageBoxStyle.Render(message))
		b.WriteString("\n\n")

		var helpText string
		if shouldUseCompactLayout(m.width, 60) {
			helpText = "Press any key..."
		} else {
			helpText = "This message will disappear in 5 seconds or press any key..."
		}
		b.WriteString(helpStyle.Render(helpText))

		return b.String()
	}

	if m.editingSlug {
		isCompact := shouldUseCompactLayout(m.width, 70)
		isVeryCompact := shouldUseCompactLayout(m.width, 50)

		var boxPadding int
		var boxMargin int
		if isVeryCompact {
			boxPadding = 1
			boxMargin = 1
		} else if isCompact {
			boxPadding = 1
			boxMargin = 1
		} else {
			boxPadding = 2
			boxMargin = 2
		}

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
			Padding(1, boxPadding).
			MarginTop(boxMargin).
			MarginBottom(boxMargin)

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
			Padding(0, boxPadding).
			MarginTop(1).
			MarginBottom(1)

		rulesBoxWidth := getResponsiveWidth(m.width, 10, 30, 60)
		rulesBoxStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, boxPadding).
			MarginTop(1).
			MarginBottom(1).
			Width(rulesBoxWidth)

		var b strings.Builder
		var title string
		if isVeryCompact {
			title = "Edit Subdomain"
		} else {
			title = "🔧 Edit Subdomain"
		}
		b.WriteString(titleStyle.Render(title))
		b.WriteString("\n\n")

		if m.tunnelType != types.HTTP {
			warningBoxWidth := getResponsiveWidth(m.width, 10, 30, 60)
			warningBoxStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFA500")).
				Background(lipgloss.Color("#3D2000")).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FFA500")).
				Padding(1, boxPadding).
				MarginTop(boxMargin).
				MarginBottom(boxMargin).
				Width(warningBoxWidth)

			var warningText string
			if isVeryCompact {
				warningText = "⚠️ TCP tunnels don't support custom subdomains."
			} else {
				warningText = "⚠️ TCP tunnels cannot have custom subdomains. Only HTTP/HTTPS tunnels support subdomain customization."
			}
			b.WriteString(warningBoxStyle.Render(warningText))
			b.WriteString("\n\n")

			var helpText string
			if isVeryCompact {
				helpText = "Press any key to go back"
			} else {
				helpText = "Press Enter or Esc to go back"
			}
			b.WriteString(helpStyle.Render(helpText))
			return b.String()
		}

		var rulesContent string
		if isVeryCompact {
			rulesContent = "Rules:\n3-20 chars\na-z, 0-9, -\nNo leading/trailing -"
		} else if isCompact {
			rulesContent = "📋 Rules:\n  • 3-20 chars\n  • a-z, 0-9, -\n  • No leading/trailing -"
		} else {
			rulesContent = "📋 Rules: \n\t• 3-20 chars \n\t• a-z, 0-9, - \n\t• No leading/trailing -"
		}
		b.WriteString(rulesBoxStyle.Render(rulesContent))
		b.WriteString("\n")

		var instruction string
		if isVeryCompact {
			instruction = "Custom subdomain:"
		} else {
			instruction = "Enter your custom subdomain:"
		}
		b.WriteString(instructionStyle.Render(instruction))
		b.WriteString("\n")

		if m.slugError != "" {
			errorInputBoxStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FF0000")).
				Padding(1, boxPadding).
				MarginTop(boxMargin).
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
		previewWidth := getResponsiveWidth(m.width, 10, 30, 80)

		if len(previewURL) > previewWidth-10 {
			previewURL = truncateString(previewURL, previewWidth-10)
		}

		previewStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Italic(true).
			Width(previewWidth)
		b.WriteString(previewStyle.Render(fmt.Sprintf("Preview: %s", previewURL)))
		b.WriteString("\n")

		var helpText string
		if isVeryCompact {
			helpText = "Enter: save • CTRL+R: random • Esc: cancel"
		} else {
			helpText = "Press Enter to save • CTRL+R for random • Esc to cancel"
		}
		b.WriteString(helpStyle.Render(helpText))

		return b.String()
	}

	if m.showingCommands {
		isCompact := shouldUseCompactLayout(m.width, 60)

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

		var title string
		if shouldUseCompactLayout(m.width, 40) {
			title = "Commands"
		} else {
			title = "⚡ Commands"
		}
		b.WriteString(titleStyle.Render(title))
		b.WriteString("\n\n")
		b.WriteString(m.commandList.View())
		b.WriteString("\n")

		var helpText string
		if isCompact {
			helpText = "↑/↓ Nav • Enter Select • Esc Cancel"
		} else {
			helpText = "↑/↓ Navigate • Enter Select • Esc Cancel"
		}
		b.WriteString(helpStyle.Render(helpText))

		return b.String()
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		PaddingTop(1)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Italic(true)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Underline(true).
		Italic(true)

	urlBoxStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		Bold(true).
		Italic(true)

	keyHintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)

	var b strings.Builder

	isCompact := shouldUseCompactLayout(m.width, 85)

	var asciiArtMargin int
	if isCompact {
		asciiArtMargin = 0
	} else {
		asciiArtMargin = 1
	}

	asciiArtStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginBottom(asciiArtMargin)

	var asciiArt string
	if shouldUseCompactLayout(m.width, 50) {
		asciiArt = "TUNNEL PLS"
	} else if isCompact {
		asciiArt = `
 ▀█▀ █ █ █▄ █ █▄ █ ██▀ █   ▄▀▀ █   ▄▀▀
  █  ▀▄█ █ ▀█ █ ▀█ █▄▄ █▄▄ ▄█▀ █▄▄ ▄█▀`
	} else {
		asciiArt = `
 ████████╗██╗   ██╗███╗   ██╗███╗   ██╗███████╗██╗         ██████╗ ██╗     ███████╗
 ╚══██╔══╝██║   ██║████╗  ██║████╗  ██║██╔════╝██║         ██╔══██╗██║     ██╔════╝
    ██║   ██║   ██║██╔██╗ ██║██╔██╗ ██║█████╗  ██║         ██████╔╝██║     ███████╗
    ██║   ██║   ██║██║╚██╗██║██║╚██╗██║██╔══╝  ██║         ██╔═══╝ ██║     ╚════██║
    ██║   ╚██████╔╝██║ ╚████║██║ ╚████║███████╗███████╗    ██║     ███████╗███████║
    ╚═╝    ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═══╝╚══════╝╚══════╝    ╚═╝     ╚══════╝╚══════╝`
	}

	b.WriteString(asciiArtStyle.Render(asciiArt))
	b.WriteString("\n")

	if !shouldUseCompactLayout(m.width, 60) {
		b.WriteString(subtitleStyle.Render("Secure tunnel service by Bagas • "))
		b.WriteString(urlStyle.Render("https://fossy.my.id"))
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n")
	}

	boxMaxWidth := getResponsiveWidth(m.width, 10, 40, 80)
	var boxPadding int
	var boxMargin int
	if isCompact {
		boxPadding = 1
		boxMargin = 1
	} else {
		boxPadding = 2
		boxMargin = 2
	}

	responsiveInfoBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(1, boxPadding).
		MarginTop(boxMargin).
		MarginBottom(boxMargin).
		Width(boxMaxWidth)

	authenticatedUser := m.interaction.lifecycle.User()

	userInfoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Bold(true)

	sectionHeaderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Bold(true)

	addressStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA"))

	var infoContent string
	if shouldUseCompactLayout(m.width, 70) {
		infoContent = fmt.Sprintf("👤 %s\n\n%s\n%s",
			userInfoStyle.Render(authenticatedUser),
			sectionHeaderStyle.Render("🌐 FORWARDING ADDRESS:"),
			addressStyle.Render(fmt.Sprintf("   %s", urlBoxStyle.Render(m.getTunnelURL()))))
	} else {
		infoContent = fmt.Sprintf("👤  Authenticated as: %s\n\n%s\n     %s",
			userInfoStyle.Render(authenticatedUser),
			sectionHeaderStyle.Render("🌐  FORWARDING ADDRESS:"),
			addressStyle.Render(urlBoxStyle.Render(m.getTunnelURL())))
	}

	b.WriteString(responsiveInfoBox.Render(infoContent))
	b.WriteString("\n")

	var quickActionsTitle string
	if shouldUseCompactLayout(m.width, 50) {
		quickActionsTitle = "Actions"
	} else if isCompact {
		quickActionsTitle = "Quick Actions"
	} else {
		quickActionsTitle = "✨ Quick Actions"
	}
	b.WriteString(titleStyle.Render(quickActionsTitle))
	b.WriteString("\n")

	var featureMargin int
	if isCompact {
		featureMargin = 1
	} else {
		featureMargin = 2
	}

	compactFeatureStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		MarginLeft(featureMargin)

	var commandsText string
	var quitText string
	if shouldUseCompactLayout(m.width, 60) {
		commandsText = fmt.Sprintf("  %s  Commands", keyHintStyle.Render("[C]"))
		quitText = fmt.Sprintf("  %s  Quit", keyHintStyle.Render("[Q]"))
	} else {
		commandsText = fmt.Sprintf("  %s  Open commands menu", keyHintStyle.Render("[C]"))
		quitText = fmt.Sprintf("  %s  Quit application", keyHintStyle.Render("[Q]"))
	}

	b.WriteString(compactFeatureStyle.Render(commandsText))
	b.WriteString("\n")
	b.WriteString(compactFeatureStyle.Render(quitText))

	if !shouldUseCompactLayout(m.width, 70) {
		b.WriteString("\n\n")
		footerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true)
		b.WriteString(footerStyle.Render("Press 'C' to customize your tunnel settings"))
	}

	return b.String()
}

func (i *interaction) Start() {
	if i.mode == types.HEADLESS {
		return
	}
	lipgloss.SetColorProfile(termenv.TrueColor)

	domain := config.Getenv("DOMAIN", "localhost")
	protocol := "http"
	if config.Getenv("TLS_ENABLED", "false") == "true" {
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
		tea.WithFPS(30),
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
	return random.GenerateRandomString(20)
}
