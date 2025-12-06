package main

import (
	"log"
	"os"
	"tunnel_pls/server"
	"tunnel_pls/utils"

	"golang.org/x/crypto/ssh"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	sshConfig := &ssh.ServerConfig{
		NoClientAuth:  true,
		ServerVersion: "SSH-2.0-TunnlPls-1.0",
	}

	privateBytes, err := os.ReadFile(utils.Getenv("ssh_private_key"))
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
