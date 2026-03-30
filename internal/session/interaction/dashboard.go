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
	isCompact := shouldUseCompactLayout(m.width, BreakpointLarge)

	var b strings.Builder
	b.WriteString(m.renderHeader(isCompact))
	b.WriteString(m.renderUserInfo(isCompact))
	b.WriteString(m.renderQuickActions(isCompact))
	b.WriteString(m.renderFooter(isCompact))

	return b.String()
}

func (m *model) renderHeader(isCompact bool) string {
	var b strings.Builder

	asciiArtMargin := getMarginValue(isCompact, 0, 1)
	asciiArtStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorPrimary)).
		MarginBottom(asciiArtMargin)

	b.WriteString(asciiArtStyle.Render(m.getASCIIArt()))
	b.WriteString("\n")

	if !shouldUseCompactLayout(m.width, BreakpointSmall) {
		b.WriteString(m.renderSubtitle())
	} else {
		b.WriteString("\n")
	}

	return b.String()
}

func (m *model) getASCIIArt() string {
	if shouldUseCompactLayout(m.width, BreakpointTiny) {
		return "TUNNEL PLS"
	}

	if shouldUseCompactLayout(m.width, BreakpointLarge) {
		return `
 ▀█▀ █ █ █▄ █ █▄ █ ██▀ █   ▄▀▀ █   ▄▀▀
  █  ▀▄█ █ ▀█ █ ▀█ █▄▄ █▄▄ ▄█▀ █▄▄ ▄█▀`
	}

	return `
 ████████╗██╗   ██╗███╗   ██╗███╗   ██╗███████╗██╗         ██████╗ ██╗     ███████╗
 ╚══██╔══╝██║   ██║████╗  ██║████╗  ██║██╔════╝██║         ██╔══██╗██║     ██╔════╝
    ██║   ██║   ██║██╔██╗ ██║██╔██╗ ██║█████╗  ██║         ██████╔╝██║     ███████╗
    ██║   ██║   ██║██║╚██╗██║██║╚██╗██║██╔══╝  ██║         ██╔═══╝ ██║     ╚════██║
    ██║   ╚██████╔╝██║ ╚████║██║ ╚████║███████╗███████╗    ██║     ███████╗███████║
    ╚═╝    ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═══╝╚══════╝╚══════╝    ╚═╝     ╚══════╝╚══════╝`
}

func (m *model) renderSubtitle() string {
	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorGray)).
		Italic(true)

	urlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorPrimary)).
		Underline(true).
		Italic(true)

	return subtitleStyle.Render("Secure tunnel service by Bagas • ") +
		urlStyle.Render("https://fossy.my.id") + "\n\n"
}

func (m *model) renderUserInfo(isCompact bool) string {
	boxMaxWidth := getResponsiveWidth(m.width, 10, 40, 80)
	boxPadding := getMarginValue(isCompact, 1, 2)
	boxMargin := getMarginValue(isCompact, 1, 2)

	responsiveInfoBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorPrimary)).
		Padding(1, boxPadding).
		MarginTop(boxMargin).
		MarginBottom(boxMargin).
		Width(boxMaxWidth)

	infoContent := m.getUserInfoContent(isCompact)
	return responsiveInfoBox.Render(infoContent) + "\n"
}

func (m *model) getUserInfoContent(isCompact bool) string {
	userInfoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWhite)).
		Bold(true)

	sectionHeaderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorGray)).
		Bold(true)

	addressStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWhite))

	urlBoxStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSecondary)).
		Bold(true).
		Italic(true)

	authenticatedUser := m.interaction.user
	tunnelURL := urlBoxStyle.Render(m.getTunnelURL())

	if isCompact {
		return fmt.Sprintf("👤 %s\n\n%s\n%s",
			userInfoStyle.Render(authenticatedUser),
			sectionHeaderStyle.Render("🌐 FORWARDING ADDRESS:"),
			addressStyle.Render(fmt.Sprintf("   %s", tunnelURL)))
	}

	return fmt.Sprintf("👤  Authenticated as: %s\n\n%s\n     %s",
		userInfoStyle.Render(authenticatedUser),
		sectionHeaderStyle.Render("🌐  FORWARDING ADDRESS:"),
		addressStyle.Render(tunnelURL))
}

func (m *model) renderQuickActions(isCompact bool) string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorPrimary)).
		PaddingTop(1)

	b.WriteString(titleStyle.Render(m.getQuickActionsTitle()))
	b.WriteString("\n")

	featureMargin := getMarginValue(isCompact, 1, 2)
	featureStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWhite)).
		MarginLeft(featureMargin)

	keyHintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorPrimary)).
		Bold(true)

	commands := m.getActionCommands(keyHintStyle)
	b.WriteString(featureStyle.Render(commands.commandsText))
	b.WriteString("\n")
	b.WriteString(featureStyle.Render(commands.quitText))

	return b.String()
}

func (m *model) getQuickActionsTitle() string {
	if shouldUseCompactLayout(m.width, BreakpointTiny) {
		return "Actions"
	}
	if shouldUseCompactLayout(m.width, BreakpointLarge) {
		return "Quick Actions"
	}
	return "✨ Quick Actions"
}

type actionCommands struct {
	commandsText string
	quitText     string
}

func (m *model) getActionCommands(keyHintStyle lipgloss.Style) actionCommands {
	if shouldUseCompactLayout(m.width, BreakpointSmall) {
		return actionCommands{
			commandsText: fmt.Sprintf("  %s  Commands", keyHintStyle.Render("[C]")),
			quitText:     fmt.Sprintf("  %s  Quit", keyHintStyle.Render("[Q]")),
		}
	}

	return actionCommands{
		commandsText: fmt.Sprintf("  %s  Open commands menu", keyHintStyle.Render("[C]")),
		quitText:     fmt.Sprintf("  %s  Quit application", keyHintStyle.Render("[Q]")),
	}
}

func (m *model) renderFooter(isCompact bool) string {
	if isCompact {
		return ""
	}

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorDarkGray)).
		Italic(true)

	return "\n\n" + footerStyle.Render("Press 'C' to customize your tunnel settings")
}

func getMarginValue(isCompact bool, compactValue, normalValue int) int {
	if isCompact {
		return compactValue
	}
	return normalValue
}
