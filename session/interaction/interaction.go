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
}

type Interaction struct {
	inputLength      int
	commandBuffer    *bytes.Buffer
	interactiveMode  bool
	interactionType  types.InteractionType
	editSlug         string
	channel          ssh.Channel
	slugManager      slug.Manager
	forwarder        Forwarder
	lifecycle        Lifecycle
	pendingExit      bool
	updateClientSlug func(oldSlug, newSlug string) bool
}

func NewInteraction(slugManager slug.Manager, forwarder Forwarder) *Interaction {
	return &Interaction{
		inputLength:      0,
		commandBuffer:    bytes.NewBuffer(make([]byte, 0, 20)),
		interactiveMode:  false,
		interactionType:  "",
		editSlug:         "",
		channel:          nil,
		slugManager:      slugManager,
		forwarder:        forwarder,
		lifecycle:        nil,
		pendingExit:      false,
		updateClientSlug: nil,
	}
}

func (i *Interaction) SetLifecycle(lifecycle Lifecycle) {
	i.lifecycle = lifecycle
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
	i.interactiveMode = false

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
	if i.interactiveMode {
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
	switch i.interactionType {
	case types.Slug:
		i.HandleSlugEditMode(char)
	}
}

func (i *Interaction) handleExitSequence(char byte) bool {
	if char == ctrlC {
		if i.pendingExit {
			i.SendMessage("Closing connection...\r\n")
			if err := i.lifecycle.Close(); err != nil {
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
	case i.commandBuffer.Len() > 0:
		i.handleCommandInput(char)
	case char == enterChar:
		i.SendMessage(clearLine)
	default:
		i.inputLength++
	}
}

func (i *Interaction) handleBackspace() {
	if i.inputLength > 0 {
		i.SendMessage(backspaceSeq)
	}
	if i.commandBuffer.Len() > 0 {
		i.commandBuffer.Truncate(i.commandBuffer.Len() - 1)
	}
}

func (i *Interaction) handleCommandStart() {
	i.commandBuffer.Reset()
	i.commandBuffer.WriteByte(forwardSlash)
}

func (i *Interaction) handleCommandInput(char byte) {
	if char == enterChar {
		i.SendMessage(clearLine)
		i.HandleCommand(i.commandBuffer.String())
		return
	}
	i.commandBuffer.WriteByte(char)
	i.inputLength++
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
	if len(i.editSlug) > 0 {
		i.editSlug = i.editSlug[:len(i.editSlug)-1]
		i.refreshSlugDisplay()
	}
}

func (i *Interaction) appendToSlug(char byte) {
	if len(i.editSlug) < maxSlugLength {
		i.editSlug += string(char)
		i.refreshSlugDisplay()
	}
}

func (i *Interaction) refreshSlugDisplay() {
	domain := utils.Getenv("DOMAIN", "localhost")
	i.SendMessage(clearToLineEnd)
	i.SendMessage("➤ " + i.editSlug + "." + domain)
}

func (i *Interaction) HandleSlugSave() {
	i.SendMessage(clearScreen)

	switch {
	case isForbiddenSlug(i.editSlug):
		i.showForbiddenSlugMessage()
	case !isValidSlug(i.editSlug):
		i.showInvalidSlugMessage()
	default:
		i.updateSlug()
	}

	i.WaitForKeyPress()
	i.returnToMainScreen()
}

func (i *Interaction) updateSlug() {
	oldSlug := i.slugManager.Get()
	newSlug := i.editSlug

	if !i.updateClientSlug(oldSlug, newSlug) {
		i.HandleSlugUpdateError()
		return
	}

	domain := utils.Getenv("DOMAIN", "localhost")
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
	i.interactiveMode = false
	i.commandBuffer.Reset()
}

func (i *Interaction) HandleSlugCancel() {
	i.SendMessage(clearScreen)
	i.SendMessage("\r\n\r\n⚠️ SUBDOMAIN EDIT CANCELLED ⚠️\r\n\r\n")
	i.SendMessage("Press any key to continue...\r\n")

	i.interactiveMode = false
	i.interactionType = ""
	i.WaitForKeyPress()

	i.SendMessage(clearScreen)
	i.ShowWelcomeMessage()
	i.ShowForwardingMessage()
}

func (i *Interaction) HandleSlugUpdateError() {
	i.SendMessage("\r\n\r\n❌ SERVER ERROR ❌\r\n\r\n")
	i.SendMessage("Failed to update subdomain. You will be disconnected in 5 seconds.\r\n\r\n")

	for countdown := 5; countdown > 0; countdown-- {
		i.SendMessage(fmt.Sprintf("Disconnecting in %d...\r\n", countdown))
		time.Sleep(1 * time.Second)
	}

	if err := i.lifecycle.Close(); err != nil {
		log.Printf("failed to close session: %v", err)
	}
}

func (i *Interaction) HandleCommand(command string) {
	handlers := map[string]func(){
		"/bye":   i.handleByeCommand,
		"/help":  i.handleHelpCommand,
		"/clear": i.handleClearCommand,
		"/slug":  i.handleSlugCommand,
	}

	if handler, exists := handlers[command]; exists {
		handler()
	} else {
		i.SendMessage("Unknown command\r\n")
	}

	i.commandBuffer.Reset()
}

func (i *Interaction) handleByeCommand() {
	i.SendMessage("Closing connection...\r\n")
	if err := i.lifecycle.Close(); err != nil {
		log.Printf("failed to close session: %v", err)
	}
}

func (i *Interaction) handleHelpCommand() {
	i.SendMessage("\r\nAvailable commands: /bye, /help, /clear, /slug\r\n")
}

func (i *Interaction) handleClearCommand() {
	i.SendMessage(clearScreen)
	i.ShowWelcomeMessage()
	i.ShowForwardingMessage()
}

func (i *Interaction) handleSlugCommand() {
	if i.forwarder.GetTunnelType() != types.HTTP {
		i.SendMessage(fmt.Sprintf("\r\n%s tunnels cannot have custom subdomains\r\n", i.forwarder.GetTunnelType()))
		return
	}

	i.interactiveMode = true
	i.interactionType = types.Slug
	i.editSlug = i.slugManager.Get()
	i.SendMessage(clearScreen)
	i.DisplaySlugEditor()

	domain := utils.Getenv("DOMAIN", "localhost")
	i.SendMessage("➤ " + i.editSlug + "." + domain)
}

func (i *Interaction) ShowForwardingMessage() {
	domain := utils.Getenv("DOMAIN", "localhost")

	if i.forwarder.GetTunnelType() == types.HTTP {
		protocol := "http"
		if utils.Getenv("TLS_ENABLED", "false") == "true" {
			protocol = "https"
		}
		i.SendMessage(fmt.Sprintf("Forwarding your traffic to %s://%s.%s \r\n", protocol, i.slugManager.Get(), domain))
	} else {
		i.SendMessage(fmt.Sprintf("Forwarding your traffic to tcp://%s:%d \r\n", domain, i.forwarder.GetForwardedPort()))
	}
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
	}

	for _, line := range asciiArt {
		i.SendMessage("\r\n" + line)
	}
	i.SendMessage("\r\n\r\n")
}

func (i *Interaction) DisplaySlugEditor() {
	domain := utils.Getenv("DOMAIN", "localhost")
	fullDomain := i.slugManager.Get() + "." + domain

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
		if err != nil {
			log.Printf("Error reading keypress: %v", err)
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
