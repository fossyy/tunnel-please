package interaction

import (
	"fmt"
	"strings"
	"tunnel_pls/internal/random"
	"tunnel_pls/types"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) slugUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.tunnelType != types.TunnelTypeHTTP {
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
		if err := m.interaction.sessionRegistry.Update(m.interaction.user, types.SessionKey{
			Id:   m.interaction.slug.String(),
			Type: types.TunnelTypeHTTP,
		}, types.SessionKey{
			Id:   inputValue,
			Type: types.TunnelTypeHTTP,
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
			newSubdomain, err := random.GenerateRandomString(20)
			if err != nil {
				return m, cmd
			}
			m.slugInput.SetValue(newSubdomain)
			m.slugError = ""
			m.slugInput, cmd = m.slugInput.Update(msg)
		}
		m.slugError = ""
		m.slugInput, cmd = m.slugInput.Update(msg)
		return m, cmd
	}
}

func (m *model) slugView() string {
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

	if m.tunnelType != types.TunnelTypeHTTP {
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
