package interaction

import (
	"fmt"
	"time"
	"tunnel_pls/internal/random"
	"tunnel_pls/types"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type commandItem struct {
	name string
	desc string
}

func (i commandItem) FilterValue() string { return i.name }
func (i commandItem) Title() string       { return i.name }
func (i commandItem) Description() string { return i.desc }

type model struct {
	randomizer        random.Random
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

const (
	ColorPrimary   = "#7D56F4"
	ColorSecondary = "#04B575"
	ColorGray      = "#888888"
	ColorDarkGray  = "#666666"
	ColorWhite     = "#FAFAFA"
	ColorError     = "#FF0000"
	ColorErrorBg   = "#3D0000"
	ColorWarning   = "#FFA500"
	ColorWarningBg = "#3D2000"
)

const (
	BreakpointTiny   = 50
	BreakpointSmall  = 60
	BreakpointMedium = 70
	BreakpointLarge  = 85
)

func (m *model) getTunnelURL() string {
	if m.tunnelType == types.TunnelTypeHTTP {
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

func (m *model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tea.WindowSize())
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

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func buildURL(protocol, subdomain, domain string) string {
	return fmt.Sprintf("%s://%s.%s", protocol, subdomain, domain)
}
