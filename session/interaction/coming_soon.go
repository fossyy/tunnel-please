package interaction

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) comingSoonUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.showingComingSoon = false
	return m, tea.Batch(tea.ClearScreen, textinput.Blink)
}

func (m *model) comingSoonView() string {
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
