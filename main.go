package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/grpc/client"
	"tunnel_pls/internal/key"
	"tunnel_pls/server"
	"tunnel_pls/session"
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

	mode := strings.ToLower(config.Getenv("MODE", "standalone"))
	isNodeMode := mode == "node"

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
		ServerVersion: fmt.Sprintf("SSH-2.0-TunnelPlease-%s", version.GetShortVersion()),
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
	sessionRegistry := session.NewRegistry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 2)
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	var grpcClient client.Client
	if isNodeMode {
		grpcHost := config.Getenv("GRPC_ADDRESS", "localhost")
		grpcPort := config.Getenv("GRPC_PORT", "8080")
		grpcAddr := fmt.Sprintf("%s:%s", grpcHost, grpcPort)
		nodeToken := config.Getenv("NODE_TOKEN", "")
		if nodeToken == "" {
			log.Fatalf("NODE_TOKEN is required in node mode")
		}

		grpcClient, err = client.New(grpcAddr, sessionRegistry)
		if err != nil {
			log.Fatalf("failed to create grpc client: %v", err)
		}

		healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
		if err = grpcClient.CheckServerHealth(healthCtx); err != nil {
			healthCancel()
			log.Fatalf("gRPC health check failed: %v", err)
		}
		healthCancel()

		go func() {
			identity := config.Getenv("DOMAIN", "localhost")
			if err = grpcClient.SubscribeEvents(ctx, identity, nodeToken); err != nil {
				errChan <- fmt.Errorf("failed to subscribe to events: %w", err)
			}
		}()
	}

	var app server.Server
	go func() {
		app, err = server.New(sshConfig, sessionRegistry, grpcClient)
		if err != nil {
			errChan <- fmt.Errorf("failed to start server: %s", err)
			return
		}
		app.Start()
	}()

	select {
	case err = <-errChan:
		log.Printf("error happen : %s", err)
	case sig := <-shutdownChan:
		log.Printf("received signal %s, shutting down", sig)
	}

	cancel()

	if app != nil {
		if err = app.Close(); err != nil {
			log.Printf("failed to close server : %s", err)
		}
	}

	if grpcClient != nil {
		if err = grpcClient.Close(); err != nil {
			log.Printf("failed to close grpc conn : %s", err)
		}
	}
}
