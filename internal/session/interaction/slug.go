package interaction

import (
	"fmt"
	"strings"
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
	case "esc", "ctrl+c":
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
	default:
		if key.Matches(msg, m.keymap.random) {
			newSubdomain, err := m.randomizer.String(20)
			if err != nil {
				return m, cmd
			}
			m.slugInput.SetValue(newSubdomain)
		}
		m.slugError = ""
		m.slugInput, cmd = m.slugInput.Update(msg)
		return m, cmd
	}
}

func (m *model) slugView() string {
	isCompact := shouldUseCompactLayout(m.width, BreakpointMedium)
	isVeryCompact := shouldUseCompactLayout(m.width, BreakpointTiny)

	var b strings.Builder
	b.WriteString(m.renderSlugTitle(isVeryCompact))

	if m.tunnelType != types.TunnelTypeHTTP {
		b.WriteString(m.renderTCPWarning(isVeryCompact, isCompact))
		return b.String()
	}

	b.WriteString(m.renderSlugRules(isVeryCompact, isCompact))
	b.WriteString(m.renderSlugInstruction(isVeryCompact))
	b.WriteString(m.renderSlugInput(isVeryCompact, isCompact))
	b.WriteString(m.renderSlugPreview(isVeryCompact))
	b.WriteString(m.renderSlugHelp(isVeryCompact))

	return b.String()
}

func (m *model) renderSlugTitle(isVeryCompact bool) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorPrimary)).
		PaddingTop(1).
		PaddingBottom(1)

	title := "🔧 Edit Subdomain"
	if isVeryCompact {
		title = "Edit Subdomain"
	}

	return titleStyle.Render(title) + "\n\n"
}

func (m *model) renderTCPWarning(isVeryCompact, isCompact bool) string {
	boxPadding := getPaddingValue(isVeryCompact, isCompact)
	boxMargin := getMarginValue(isCompact, 1, 2)
	warningBoxWidth := getResponsiveWidth(m.width, 10, 30, 60)

	warningBoxStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWarning)).
		Background(lipgloss.Color(ColorWarningBg)).
		Bold(true).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorWarning)).
		Padding(1, boxPadding).
		MarginTop(boxMargin).
		MarginBottom(boxMargin).
		Width(warningBoxWidth)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorDarkGray)).
		Italic(true).
		MarginTop(1)

	warningText := m.getTCPWarningText(isVeryCompact)
	helpText := m.getTCPHelpText(isVeryCompact)

	var b strings.Builder
	b.WriteString(warningBoxStyle.Render(warningText))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render(helpText))

	return b.String()
}

func (m *model) getTCPWarningText(isVeryCompact bool) string {
	if isVeryCompact {
		return "⚠️ TCP tunnels don't support custom subdomains."
	}
	return "⚠️ TCP tunnels cannot have custom subdomains. Only HTTP/HTTPS tunnels support subdomain customization."
}

func (m *model) getTCPHelpText(isVeryCompact bool) string {
	if isVeryCompact {
		return "Press any key to go back"
	}
	return "Press Enter or Esc to go back"
}

func (m *model) renderSlugRules(isVeryCompact, isCompact bool) string {
	boxPadding := getPaddingValue(isVeryCompact, isCompact)
	rulesBoxWidth := getResponsiveWidth(m.width, 10, 30, 60)

	rulesBoxStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWhite)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorPrimary)).
		Padding(0, boxPadding).
		MarginTop(1).
		MarginBottom(1).
		Width(rulesBoxWidth)

	rulesContent := m.getRulesContent(isVeryCompact, isCompact)
	return rulesBoxStyle.Render(rulesContent) + "\n"
}

func (m *model) getRulesContent(isVeryCompact, isCompact bool) string {
	if isVeryCompact {
		return "Rules:\n3-20 chars\na-z, 0-9, -\nNo leading/trailing -"
	}

	if isCompact {
		return "📋 Rules:\n  • 3-20 chars\n  • a-z, 0-9, -\n  • No leading/trailing -"
	}

	return "📋 Rules: \n\t• 3-20 chars \n\t• a-z, 0-9, - \n\t• No leading/trailing -"
}

func (m *model) renderSlugInstruction(isVeryCompact bool) string {
	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWhite)).
		MarginTop(1)

	instruction := "Enter your custom subdomain:"
	if isVeryCompact {
		instruction = "Custom subdomain:"
	}

	return instructionStyle.Render(instruction) + "\n"
}

func (m *model) renderSlugInput(isVeryCompact, isCompact bool) string {
	boxPadding := getPaddingValue(isVeryCompact, isCompact)
	boxMargin := getMarginValue(isCompact, 1, 2)

	if m.slugError != "" {
		return m.renderErrorInput(boxPadding, boxMargin)
	}

	return m.renderNormalInput(boxPadding, boxMargin)
}

func (m *model) renderErrorInput(boxPadding, boxMargin int) string {
	errorInputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorError)).
		Padding(1, boxPadding).
		MarginTop(boxMargin).
		MarginBottom(1)

	errorBoxStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorError)).
		Background(lipgloss.Color(ColorErrorBg)).
		Bold(true).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorError)).
		Padding(0, boxPadding).
		MarginTop(1).
		MarginBottom(1)

	var b strings.Builder
	b.WriteString(errorInputBoxStyle.Render(m.slugInput.View()))
	b.WriteString("\n")
	b.WriteString(errorBoxStyle.Render("❌ " + m.slugError))
	b.WriteString("\n")

	return b.String()
}

func (m *model) renderNormalInput(boxPadding, boxMargin int) string {
	inputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorPrimary)).
		Padding(1, boxPadding).
		MarginTop(boxMargin).
		MarginBottom(boxMargin)

	return inputBoxStyle.Render(m.slugInput.View()) + "\n"
}

func (m *model) renderSlugPreview(isVeryCompact bool) string {
	previewURL := buildURL(m.protocol, m.slugInput.Value(), m.domain)
	previewWidth := getResponsiveWidth(m.width, 10, 30, 80)

	if isVeryCompact {
		previewURL = truncateString(previewURL, previewWidth-10)
	}

	previewStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSecondary)).
		Italic(true).
		Width(previewWidth)

	return previewStyle.Render(fmt.Sprintf("Preview: %s", previewURL)) + "\n"
}

func (m *model) renderSlugHelp(isVeryCompact bool) string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorDarkGray)).
		Italic(true).
		MarginTop(1)

	helpText := "Press Enter to save • CTRL+R for random • Esc to cancel"
	if isVeryCompact {
		helpText = "Enter: save • CTRL+R: random • Esc: cancel"
	}

	return helpStyle.Render(helpText)
}

func getPaddingValue(isVeryCompact, isCompact bool) int {
	if isVeryCompact || isCompact {
		return 1
	}
	return 2
}
