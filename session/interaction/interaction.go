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

type InteractionController interface {
	SendMessage(message string)
	HandleUserInput()
	HandleCommand(command string, commandBuffer *bytes.Buffer)
	HandleSlugEditMode(connection ssh.Channel, char byte, commandBuffer *bytes.Buffer)
	HandleSlugSave(conn ssh.Channel)
	HandleSlugCancel(connection ssh.Channel, commandBuffer *bytes.Buffer)
	HandleSlugUpdateError()
	ShowWelcomeMessage()
	DisplaySlugEditor()
	SetChannel(channel ssh.Channel)
	SetLifecycle(lifecycle Lifecycle)
}

type Forwarder interface {
	Close() error
	GetTunnelType() types.TunnelType
	GetForwardedPort() uint16
}

type Interaction struct {
	CommandBuffer *bytes.Buffer
	EditMode      bool
	EditSlug      string
	channel       ssh.Channel
	SlugManager   slug.Manager
	Forwarder     Forwarder
	Lifecycle     Lifecycle
}

func (i *Interaction) SetLifecycle(lifecycle Lifecycle) {
	i.Lifecycle = lifecycle
}

func (i *Interaction) SetChannel(channel ssh.Channel) {
	i.channel = channel
}

func (i *Interaction) SendMessage(message string) {
	if i.channel != nil {
		_, err := i.channel.Write([]byte(message))
		if err != nil && err != io.EOF {
			log.Printf("Error writing to channel: %v", err)
			return
		}
	}
}

func (i *Interaction) HandleUserInput() {
	var commandBuffer bytes.Buffer
	buf := make([]byte, 1)
	i.EditMode = false

	for {
		n, err := i.channel.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading from client: %s", err)
			}
			break
		}

		if n > 0 {
			char := buf[0]

			if i.EditMode {
				i.HandleSlugEditMode(i.channel, char, &commandBuffer)
				continue
			}

			i.SendMessage(string(buf[:n]))

			if char == 8 || char == 127 {
				if commandBuffer.Len() > 0 {
					commandBuffer.Truncate(commandBuffer.Len() - 1)
					i.SendMessage("\b \b")
				}
				continue
			}

			if char == '/' {
				commandBuffer.Reset()
				commandBuffer.WriteByte(char)
				continue
			}

			if commandBuffer.Len() > 0 {
				if char == 13 {
					i.HandleCommand(commandBuffer.String(), &commandBuffer)
					continue
				}
				commandBuffer.WriteByte(char)
			}
		}
	}
}

func (i *Interaction) HandleSlugEditMode(connection ssh.Channel, char byte, commandBuffer *bytes.Buffer) {
	if char == 13 {
		i.HandleSlugSave(connection)
	} else if char == 27 {
		i.HandleSlugCancel(connection, commandBuffer)
	} else if char == 8 || char == 127 {
		if len(i.EditSlug) > 0 {
			i.EditSlug = (i.EditSlug)[:len(i.EditSlug)-1]
			_, err := connection.Write([]byte("\r\033[K"))
			if err != nil {
				log.Printf("failed to write to channel: %v", err)
				return
			}
			_, err = connection.Write([]byte("➤ " + i.EditSlug + "." + utils.Getenv("domain")))
			if err != nil {
				log.Printf("failed to write to channel: %v", err)
				return
			}
		}
	} else if char >= 32 && char <= 126 {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			i.EditSlug += string(char)
			_, err := connection.Write([]byte("\r\033[K"))
			if err != nil {
				log.Printf("failed to write to channel: %v", err)
				return
			}
			_, err = connection.Write([]byte("➤ " + i.EditSlug + "." + utils.Getenv("domain")))
			if err != nil {
				log.Printf("failed to write to channel: %v", err)
				return
			}
		}
	}
}

func (i *Interaction) HandleSlugSave(connection ssh.Channel) {
	isValid := isValidSlug(i.EditSlug)

	_, err := connection.Write([]byte("\033[H\033[2J"))
	if err != nil {
		log.Printf("failed to write to channel: %v", err)
		return
	}
	if isValid {
		//oldSlug := i.SlugManager.Get()
		newSlug := i.EditSlug

		//if !i.updateClientSlug(oldSlug, newSlug) {
		//	i.HandleSlugUpdateError()
		//	return
		//}

		_, err := connection.Write([]byte("\r\n\r\n✅ SUBDOMAIN UPDATED ✅\r\n\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
		_, err = connection.Write([]byte("Your new address is: " + newSlug + "." + utils.Getenv("domain") + "\r\n\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
		_, err = connection.Write([]byte("Press any key to continue...\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
	} else if isForbiddenSlug(i.EditSlug) {
		_, err := connection.Write([]byte("\r\n\r\n❌ FORBIDDEN SUBDOMAIN ❌\r\n\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
		_, err = connection.Write([]byte("This subdomain is not allowed.\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
		_, err = connection.Write([]byte("Please try a different subdomain.\r\n\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
		_, err = connection.Write([]byte("Press any key to continue...\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
	} else {
		_, err := connection.Write([]byte("\r\n\r\n❌ INVALID SUBDOMAIN ❌\r\n\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
		_, err = connection.Write([]byte("Use only lowercase letters, numbers, and hyphens.\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
		_, err = connection.Write([]byte("Length must be 3-20 characters and cannot start or end with a hyphen.\r\n\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
		_, err = connection.Write([]byte("Press any key to continue...\r\n"))
		if err != nil {
			log.Printf("failed to write to channel: %v", err)
			return
		}
	}

	waitForKeyPress(connection)

	_, err = connection.Write([]byte("\033[H\033[2J"))
	if err != nil {
		log.Printf("failed to write to channel: %v", err)
		return
	}
	i.ShowWelcomeMessage()

	domain := utils.Getenv("domain")
	protocol := "http"
	if utils.Getenv("tls_enabled") == "true" {
		protocol = "https"
	}
	_, err = connection.Write([]byte(fmt.Sprintf("Forwarding your traffic to %s://%s.%s \r\n", protocol, i.SlugManager.Get(), domain)))
	if err != nil {
		log.Printf("failed to write to channel: %v", err)
		return
	}

	i.EditMode = false
	i.CommandBuffer.Reset()
}

func (i *Interaction) HandleSlugCancel(connection ssh.Channel, commandBuffer *bytes.Buffer) {
	i.EditMode = false
	_, err := connection.Write([]byte("\033[H\033[2J"))
	if err != nil {
		log.Printf("failed to write to channel: %v", err)
		return
	}
	_, err = connection.Write([]byte("\r\n\r\n⚠️ SUBDOMAIN EDIT CANCELLED ⚠️\r\n\r\n"))
	if err != nil {
		log.Printf("failed to write to channel: %v", err)
		return
	}
	_, err = connection.Write([]byte("Press any key to continue...\r\n"))
	if err != nil {
		log.Printf("failed to write to channel: %v", err)
		return
	}

	waitForKeyPress(connection)

	_, err = connection.Write([]byte("\033[H\033[2J"))
	if err != nil {
		log.Printf("failed to write to channel: %v", err)
		return
	}
	i.ShowWelcomeMessage()

	commandBuffer.Reset()
}

func (i *Interaction) HandleSlugUpdateError() {
	i.SendMessage("\r\n\r\n❌ SERVER ERROR ❌\r\n\r\n")
	i.SendMessage("Failed to update subdomain. You will be disconnected in 5 seconds.\r\n\r\n")

	for iter := 5; iter > 0; iter-- {
		i.SendMessage(fmt.Sprintf("Disconnecting in %d...\r\n", iter))
		time.Sleep(1 * time.Second)
	}
	err := i.Lifecycle.Close()
	if err != nil {
		log.Printf("failed to close session: %v", err)
		return
	}
}

func (i *Interaction) HandleCommand(command string, commandBuffer *bytes.Buffer) {
	switch command {
	case "/bye":
		i.SendMessage("\r\nClosing connection...")
		err := i.Lifecycle.Close()
		if err != nil {
			log.Printf("failed to close session: %v", err)
			return
		}
		return
	case "/help":
		i.SendMessage("\r\nAvailable commands: /bye, /help, /clear, /slug")
	case "/clear":
		i.SendMessage("\033[H\033[2J")
		i.ShowWelcomeMessage()
		domain := utils.Getenv("domain")
		if i.Forwarder.GetTunnelType() == types.HTTP {
			protocol := "http"
			if utils.Getenv("tls_enabled") == "true" {
				protocol = "https"
			}
			i.SendMessage(fmt.Sprintf("Forwarding your traffic to %s://%s.%s \r\n", protocol, i.SlugManager.Get(), domain))
		} else {
			i.SendMessage(fmt.Sprintf("Forwarding your traffic to %s://%s:%d \r\n", i.Forwarder.GetTunnelType(), domain, i.Forwarder.GetForwardedPort()))
		}
	case "/slug":
		if i.Forwarder.GetTunnelType() != types.HTTP {
			i.SendMessage((fmt.Sprintf("\r\n%s tunnels cannot have custom subdomains", i.Forwarder.GetTunnelType())))
		} else {
			i.EditMode = true
			i.EditSlug = i.SlugManager.Get()
			i.SendMessage("\033[H\033[2J")
			i.DisplaySlugEditor()
			i.SendMessage("➤ " + i.EditSlug + "." + utils.Getenv("domain"))
		}
	default:
		i.SendMessage("Unknown command")
	}

	commandBuffer.Reset()
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
	domain := utils.Getenv("domain")
	fullDomain := i.SlugManager.Get() + "." + domain

	const paddingRight = 4

	contentLine := "  ║  Current:  " + fullDomain
	boxWidth := len(contentLine) + paddingRight + 1
	if boxWidth < 50 {
		boxWidth = 50
	}

	topBorder := "  ╔" + strings.Repeat("═", boxWidth-4) + "╗\r\n"
	title := centerText("SUBDOMAIN EDITOR", boxWidth-4)
	header := "  ║" + title + "║\r\n"
	midBorder := "  ╠" + strings.Repeat("═", boxWidth-4) + "╣\r\n"
	emptyLine := "  ║" + strings.Repeat(" ", boxWidth-4) + "║\r\n"

	currentLineContent := fmt.Sprintf("  ║  Current:  %s", fullDomain)
	currentLine := currentLineContent + strings.Repeat(" ", boxWidth-len(currentLineContent)+1) + "║\r\n"

	saveCancel := "  ║  [Enter] Save  |  [Esc] Cancel" + strings.Repeat(" ", boxWidth-35) + "║\r\n"
	bottomBorder := "  ╚" + strings.Repeat("═", boxWidth-4) + "╝\r\n"

	i.SendMessage("\r\n\r\n")
	i.SendMessage(topBorder)
	i.SendMessage(header)
	i.SendMessage(midBorder)
	i.SendMessage(emptyLine)
	i.SendMessage(currentLine)
	i.SendMessage(emptyLine)
	i.SendMessage(emptyLine)
	i.SendMessage(midBorder)
	i.SendMessage(saveCancel)
	i.SendMessage(bottomBorder)
	i.SendMessage("\r\n\r\n")
}

func centerText(text string, width int) string {
	padding := (width - len(text)) / 2
	if padding < 0 {
		padding = 0
	}
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-len(text)-padding)
}
func isValidSlug(slug string) bool {
	if len(slug) < 3 || len(slug) > 20 {
		return false
	}

	if slug[0] == '-' || slug[len(slug)-1] == '-' {
		return false
	}

	for _, c := range slug {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}

	return true
}

func waitForKeyPress(connection ssh.Channel) {
	keyBuf := make([]byte, 1)
	for {
		_, err := connection.Read(keyBuf)
		if err == nil {
			break
		}
	}
}

var forbiddenSlug = []string{
	"ping",
}

func isForbiddenSlug(slug string) bool {
	for _, s := range forbiddenSlug {
		if slug == s {
			return true
		}
	}
	return false
}
