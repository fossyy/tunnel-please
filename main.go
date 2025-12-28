package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"tunnel_pls/server"
	"tunnel_pls/utils"

	"golang.org/x/crypto/ssh"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	pprofEnabled := utils.Getenv("PPROF_ENABLED", "false")
	if pprofEnabled == "true" {
		pprofPort := utils.Getenv("PPROF_PORT", "6060")
		go func() {
			pprofAddr := fmt.Sprintf("localhost:%s", pprofPort)
			log.Printf("Starting pprof server on http://%s/debug/pprof/", pprofAddr)
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				log.Printf("pprof server error: %v", err)
			}
		}()
	}

	sshConfig := &ssh.ServerConfig{
		NoClientAuth:  true,
		ServerVersion: "SSH-2.0-TunnlPls-1.0",
	}

	sshKeyPath := utils.Getenv("SSH_PRIVATE_KEY", "certs/id_rsa")
	if err := utils.GenerateSSHKeyIfNotExist(sshKeyPath); err != nil {
		log.Fatalf("Failed to generate SSH key: %s", err)
	}

	privateBytes, err := os.ReadFile(sshKeyPath)
	if err != nil {
		log.Fatalf("Failed to load private key: %s", err)
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatalf("Failed to parse private key: %s", err)
	}

	sshConfig.AddHostKey(private)
	app := server.NewServer(sshConfig)
	app.Start()
}
