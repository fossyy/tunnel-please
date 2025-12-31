package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/key"
	"tunnel_pls/server"
	"tunnel_pls/version"

	"golang.org/x/crypto/ssh"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(version.GetVersion())
		os.Exit(0)
	}

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Printf("Starting %s", version.GetVersion())

	pprofEnabled := config.Getenv("PPROF_ENABLED", "false")
	if pprofEnabled == "true" {
		pprofPort := config.Getenv("PPROF_PORT", "6060")
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
		ServerVersion: fmt.Sprintf("SSH-2.0-TunnlPls-%s", version.GetShortVersion()),
	}

	sshKeyPath := "certs/ssh/id_rsa"
	if err := key.GenerateSSHKeyIfNotExist(sshKeyPath); err != nil {
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
