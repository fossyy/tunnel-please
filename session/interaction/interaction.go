package interaction

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
	"time"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"
	"tunnel_pls/utils"

	"golang.org/x/crypto/ssh"
)

type Lifecycle interface {
	Close() error
}

type Controller interface {
	SendMessage(message string)
	HandleUserInput()
	HandleCommand(command string)
	HandleSlugEditMode(char byte)
	HandleSlugSave()
	HandleSlugCancel()
	HandleSlugUpdateError()
	ShowWelcomeMessage()
	DisplaySlugEditor()
	SetChannel(channel ssh.Channel)
	SetLifecycle(lifecycle Lifecycle)
	SetSlugModificator(func(oldSlug, newSlug string) bool)
	WaitForKeyPress()
	ShowForwardingMessage()
}

type Forwarder interface {
	Close() error
	GetTunnelType() types.TunnelType
	GetForwardedPort() uint16
	DropAllForwarder() int
	GetForwarderCount() int
}

type Interaction struct {
	InputLength      int
	CommandBuffer    *bytes.Buffer
	InteractiveMode  bool
	InteractionType  types.InteractionType
	EditSlug         string
	channel          ssh.Channel
	SlugManager      slug.Manager
	Forwarder        Forwarder
	Lifecycle        Lifecycle
	pendingExit      bool
	updateClientSlug func(oldSlug, newSlug string) bool
}

func (i *Interaction) SetLifecycle(lifecycle Lifecycle) {
	i.Lifecycle = lifecycle
}

func (i *Interaction) SetChannel(channel ssh.Channel) {
	i.channel = channel
}

func (i *Interaction) SendMessage(message string) {
	if i.channel == nil {
		log.Printf("channel is nil")
	}

	_, err := i.channel.Write([]byte(message))
	if err != nil && err != io.EOF {
		log.Printf("error writing to channel: %s", err)
	}
	return
}

func (i *Interaction) HandleUserInput() {
	buf := make([]byte, 1)
	i.InteractiveMode = false

	for {
		n, err := i.channel.Read(buf)
		if err != nil {
			i.handleReadError(err)
			break
		}

		if n > 0 {
			i.processCharacter(buf[0])
		}
	}
}

func (i *Interaction) handleReadError(err error) {
	if err != io.EOF {
		log.Printf("Error reading from client: %s", err)
	}
}

func (i *Interaction) processCharacter(char byte) {
	if i.InteractiveMode {
		i.handleInteractiveMode(char)
		return
	}

	if i.handleExitSequence(char) {
		return
	}

	i.SendMessage(string(char))
	i.handleNonInteractiveInput(char)
}

func (i *Interaction) handleInteractiveMode(char byte) {
	switch i.InteractionType {
	case types.Slug:
		i.HandleSlugEditMode(char)
	case types.Drop:
		i.HandleDropMode(char)
	}
}

func (i *Interaction) handleExitSequence(char byte) bool {
	if char == ctrlC {
		if i.pendingExit {
			i.SendMessage("Closing connection...\r\n")
			if err := i.Lifecycle.Close(); err != nil {
				log.Printf("failed to close session: %v", err)
			}
			return true
		}
		i.SendMessage("Please press Ctrl+C again to disconnect.\r\n")
		i.pendingExit = true
		return true
	}

	if i.pendingExit && char != ctrlC {
		i.pendingExit = false
		i.SendMessage("Operation canceled.\r\n")
	}

	return false
}

func (i *Interaction) handleNonInteractiveInput(char byte) {
	switch {
	case char == backspaceChar || char == deleteChar:
		i.handleBackspace()
	case char == forwardSlash:
		i.handleCommandStart()
	case i.CommandBuffer.Len() > 0:
		i.handleCommandInput(char)
	case char == enterChar:
		i.SendMessage(clearLine)
	default:
		i.InputLength++
	}
}

func (i *Interaction) handleBackspace() {
	if i.InputLength > 0 {
		i.SendMessage(backspaceSeq)
	}
	if i.CommandBuffer.Len() > 0 {
		i.CommandBuffer.Truncate(i.CommandBuffer.Len() - 1)
	}
}

func (i *Interaction) handleCommandStart() {
	i.CommandBuffer.Reset()
	i.CommandBuffer.WriteByte(forwardSlash)
}

func (i *Interaction) handleCommandInput(char byte) {
	if char == enterChar {
		i.SendMessage(clearLine)
		i.HandleCommand(i.CommandBuffer.String())
		return
	}
	i.CommandBuffer.WriteByte(char)
	i.InputLength++
}

func (i *Interaction) HandleSlugEditMode(char byte) {
	switch {
	case char == enterChar:
		i.HandleSlugSave()
	case char == escapeChar || char == ctrlC:
		i.HandleSlugCancel()
	case char == backspaceChar || char == deleteChar:
		i.handleSlugBackspace()
	case char >= minPrintableChar && char <= maxPrintableChar:
		i.appendToSlug(char)
	}
}

func (i *Interaction) handleSlugBackspace() {
	if len(i.EditSlug) > 0 {
		i.EditSlug = i.EditSlug[:len(i.EditSlug)-1]
		i.refreshSlugDisplay()
	}
}

func (i *Interaction) appendToSlug(char byte) {
	if isValidSlugChar(char) {
		i.EditSlug += string(char)
		i.refreshSlugDisplay()
	}
}

func (i *Interaction) refreshSlugDisplay() {
	domain := utils.Getenv("domain")
	i.SendMessage(clearToLineEnd)
	i.SendMessage("➤ " + i.EditSlug + "." + domain)
}

func (i *Interaction) HandleSlugSave() {
	i.SendMessage(clearScreen)

	switch {
	case isForbiddenSlug(i.EditSlug):
		i.showForbiddenSlugMessage()
	case !isValidSlug(i.EditSlug):
		i.showInvalidSlugMessage()
	default:
		i.updateSlug()
	}

	i.WaitForKeyPress()
	i.returnToMainScreen()
}

func (i *Interaction) updateSlug() {
	oldSlug := i.SlugManager.Get()
	newSlug := i.EditSlug

	if !i.updateClientSlug(oldSlug, newSlug) {
		i.HandleSlugUpdateError()
		return
	}

	domain := utils.Getenv("domain")
	i.SendMessage("\r\n\r\n✅ SUBDOMAIN UPDATED ✅\r\n\r\n")
	i.SendMessage("Your new address is: " + newSlug + "." + domain + "\r\n\r\n")
	i.SendMessage("Press any key to continue...\r\n")
}

func (i *Interaction) showForbiddenSlugMessage() {
	i.SendMessage("\r\n\r\n❌ FORBIDDEN SUBDOMAIN ❌\r\n\r\n")
	i.SendMessage("This subdomain is not allowed.\r\n")
	i.SendMessage("Please try a different subdomain.\r\n\r\n")
	i.SendMessage("Press any key to continue...\r\n")
}

func (i *Interaction) showInvalidSlugMessage() {
	i.SendMessage("\r\n\r\n❌ INVALID SUBDOMAIN ❌\r\n\r\n")
	i.SendMessage("Use only lowercase letters, numbers, and hyphens.\r\n")
	i.SendMessage(fmt.Sprintf("Length must be %d-%d characters and cannot start or end with a hyphen.\r\n\r\n", minSlugLength, maxSlugLength))
	i.SendMessage("Press any key to continue...\r\n")
}

func (i *Interaction) returnToMainScreen() {
	i.SendMessage(clearScreen)
	i.ShowWelcomeMessage()
	i.ShowForwardingMessage()
	i.InteractiveMode = false
	i.CommandBuffer.Reset()
}

func (i *Interaction) HandleSlugCancel() {
	i.InteractiveMode = false
	i.showMessageAndWait("\r\n\r\n⚠️ SUBDOMAIN EDIT CANCELLED ⚠️\r\n\r\n")
}

func (i *Interaction) HandleSlugUpdateError() {
	i.SendMessage("\r\n\r\n❌ SERVER ERROR ❌\r\n\r\n")
	i.SendMessage("Failed to update subdomain. You will be disconnected in 5 seconds.\r\n\r\n")

	for countdown := 5; countdown > 0; countdown-- {
		i.SendMessage(fmt.Sprintf("Disconnecting in %d...\r\n", countdown))
		time.Sleep(1 * time.Second)
	}

	if err := i.Lifecycle.Close(); err != nil {
		log.Printf("failed to close session: %v", err)
	}
}

func (i *Interaction) HandleCommand(command string) {
	handlers := map[string]func(){
		"/bye":   i.handleByeCommand,
		"/help":  i.handleHelpCommand,
		"/clear": i.handleClearCommand,
		"/slug":  i.handleSlugCommand,
		"/drop":  i.handleDropCommand,
	}

	if handler, exists := handlers[command]; exists {
		handler()
	} else {
		i.SendMessage("Unknown command\r\n")
	}

	i.CommandBuffer.Reset()
}

func (i *Interaction) handleByeCommand() {
	i.SendMessage("Closing connection...\r\n")
	if err := i.Lifecycle.Close(); err != nil {
		log.Printf("failed to close session: %v", err)
	}
}

func (i *Interaction) handleHelpCommand() {
	i.SendMessage("\r\nAvailable commands: /bye, /help, /clear, /slug, /drop\r\n")
}

func (i *Interaction) handleClearCommand() {
	i.SendMessage(clearScreen)
	i.ShowWelcomeMessage()
	i.ShowForwardingMessage()
}

func (i *Interaction) handleSlugCommand() {
	if i.Forwarder.GetTunnelType() != types.HTTP {
		i.SendMessage(fmt.Sprintf("\r\n%s tunnels cannot have custom subdomains\r\n", i.Forwarder.GetTunnelType()))
		return
	}

	i.InteractiveMode = true
	i.InteractionType = types.Slug
	i.EditSlug = i.SlugManager.Get()
	i.SendMessage(clearScreen)
	i.DisplaySlugEditor()

	domain := utils.Getenv("domain")
	i.SendMessage("➤ " + i.EditSlug + "." + domain)
}

func (i *Interaction) handleDropCommand() {
	i.InteractiveMode = true
	i.InteractionType = types.Drop
	i.SendMessage(clearScreen)
	i.ShowDropMessage()
}

func (i *Interaction) ShowForwardingMessage() {
	domain := utils.Getenv("domain")

	if i.Forwarder.GetTunnelType() == types.HTTP {
		protocol := "http"
		if utils.Getenv("tls_enabled") == "true" {
			protocol = "https"
		}
		i.SendMessage(fmt.Sprintf("Forwarding your traffic to %s://%s.%s \r\n", protocol, i.SlugManager.Get(), domain))
	} else {
		i.SendMessage(fmt.Sprintf("Forwarding your traffic to tcp://%s:%d \r\n", domain, i.Forwarder.GetForwardedPort()))
	}
}

func (i *Interaction) HandleDropMode(char byte) {
	switch {
	case char == enterChar || char == 'y' || char == 'Y':
		i.executeDropAll()
	case char == escapeChar || char == 'n' || char == 'N' || char == ctrlC:
		i.cancelDrop()
	}
}

func (i *Interaction) executeDropAll() {
	count := i.Forwarder.DropAllForwarder()
	message := fmt.Sprintf("Dropped %d forwarders\r\n", count)
	i.showMessageAndWait(message)
}

func (i *Interaction) cancelDrop() {
	i.showMessageAndWait("Dropping canceled.\r\n")
}

func (i *Interaction) showMessageAndWait(message string) {
	i.SendMessage(clearScreen)
	i.SendMessage(message)
	i.SendMessage("Press any key to continue...\r\n")

	i.InteractiveMode = false
	i.InteractionType = ""
	i.WaitForKeyPress()

	i.SendMessage(clearScreen)
	i.ShowWelcomeMessage()
	i.ShowForwardingMessage()
}

func (i *Interaction) ShowDropMessage() {
	confirmText := fmt.Sprintf("  ║  Drop ALL %d active connections?", i.Forwarder.GetForwarderCount())
	boxWidth := calculateBoxWidth(confirmText)

	box := buildDropConfirmationBox(boxWidth, confirmText)
	i.SendMessage("\r\n" + box + "\r\n\r\n")
}

func buildDropConfirmationBox(boxWidth int, confirmText string) string {
	topBorder := "  ╔" + strings.Repeat("═", boxWidth-4) + "╗\r\n"
	title := centerText("DROP CONFIRMATION", boxWidth-4)
	header := "  ║" + title + "║\r\n"
	midBorder := "  ╠" + strings.Repeat("═", boxWidth-4) + "╣\r\n"
	emptyLine := "  ║" + strings.Repeat(" ", boxWidth-4) + "║\r\n"

	confirmLine := confirmText + strings.Repeat(" ", boxWidth-len(confirmText)+1) + "║\r\n"

	controlText := "  ║  [Enter/Y] Confirm    [N/Esc] Cancel"
	controlLine := controlText + strings.Repeat(" ", boxWidth-len(controlText)+1) + "║\r\n"

	bottomBorder := "  ╚" + strings.Repeat("═", boxWidth-4) + "╝\r\n"

	return topBorder + header + midBorder + emptyLine + confirmLine + emptyLine + controlLine + emptyLine + bottomBorder
}

func (i *Interaction) ShowWelcomeMessage() {
	asciiArt := []string{
		` _______                     _   _____  _      `,
		`|__   __|                   | | |  __ \| |    `,
		`   | |_   _ _ __  _ __   ___| | | |__) | |___ `,
		`   | | | | | '_ \| '_ \ / _ \ | |  ___/| / __|`,
		`   | | |_| | | | | | | |  __/ | | |    | \__ \`,
		`   |_|\__,_|_| |_|_| |_|\___|_| |_|    |_|___/`,
		``,
		`       "Tunnel Pls" - Project by Bagas`,
		`           https://fossy.my.id`,
		``,
		`        Welcome to Tunnel! Available commands:`,
		`        - '/bye'   : Exit the tunnel`,
		`        - '/help'  : Show this help message`,
		`        - '/clear' : Clear the current line`,
		`        - '/slug'  : Set custom subdomain`,
		`        - '/drop'  : Drop all active forwarders`,
	}

	for _, line := range asciiArt {
		i.SendMessage("\r\n" + line)
	}
	i.SendMessage("\r\n\r\n")
}

func (i *Interaction) DisplaySlugEditor() {
	domain := utils.Getenv("domain")
	fullDomain := i.SlugManager.Get() + "." + domain

	contentLine := "  ║  Current:  " + fullDomain
	boxWidth := calculateBoxWidth(contentLine)

	box := buildSlugEditorBox(boxWidth, fullDomain)
	i.SendMessage("\r\n\r\n" + box + "\r\n\r\n")
}

func buildSlugEditorBox(boxWidth int, fullDomain string) string {
	topBorder := "  ╔" + strings.Repeat("═", boxWidth-4) + "╗\r\n"
	title := centerText("SUBDOMAIN EDITOR", boxWidth-4)
	header := "  ║" + title + "║\r\n"
	midBorder := "  ╠" + strings.Repeat("═", boxWidth-4) + "╣\r\n"
	emptyLine := "  ║" + strings.Repeat(" ", boxWidth-4) + "║\r\n"

	currentLineContent := fmt.Sprintf("  ║  Current:  %s", fullDomain)
	currentLine := currentLineContent + strings.Repeat(" ", boxWidth-len(currentLineContent)+1) + "║\r\n"

	saveCancel := "  ║  [Enter] Save  |  [Esc] Cancel" + strings.Repeat(" ", boxWidth-35) + "║\r\n"
	bottomBorder := "  ╚" + strings.Repeat("═", boxWidth-4) + "╝\r\n"

	return topBorder + header + midBorder + emptyLine + currentLine + emptyLine + emptyLine + midBorder + saveCancel + bottomBorder
}

func (i *Interaction) SetSlugModificator(modificator func(oldSlug, newSlug string) bool) {
	i.updateClientSlug = modificator
}

func (i *Interaction) WaitForKeyPress() {
	keyBuf := make([]byte, 1)
	for {
		_, err := i.channel.Read(keyBuf)
		if err == nil {
			break
		}
	}
}

func calculateBoxWidth(contentLine string) int {
	boxWidth := len(contentLine) + paddingRight + 1
	if boxWidth < minBoxWidth {
		boxWidth = minBoxWidth
	}
	return boxWidth
}

func centerText(text string, width int) string {
	padding := (width - len(text)) / 2
	if padding < 0 {
		padding = 0
	}
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-len(text)-padding)
}

func isValidSlug(slug string) bool {
	if len(slug) < minSlugLength || len(slug) > maxSlugLength {
		return false
	}

	if slug[0] == '-' || slug[len(slug)-1] == '-' {
		return false
	}

	for _, c := range slug {
		if !isValidSlugChar(byte(c)) {
			return false
		}
	}

	return true
}

func isValidSlugChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
}

func isForbiddenSlug(slug string) bool {
	for _, s := range forbiddenSlugs {
		if slug == s {
			return true
		}
	}
	return false
}
