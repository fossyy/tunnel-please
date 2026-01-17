package interaction

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) dashboardUpdate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keymap.quit):
		m.quitting = true
		return m, tea.Batch(tea.ClearScreen, textinput.Blink, tea.Quit)
	case key.Matches(msg, m.keymap.command):
		m.showingCommands = true
		return m, tea.Batch(tea.ClearScreen, textinput.Blink)
	}
	return m, nil
}

func (m *model) dashboardView() string {
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
