package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
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
	sessionRegistry := session.NewRegistry()

	grpcClient, err := client.New(&client.GrpcConfig{
		Address:            "localhost:8080",
		UseTLS:             false,
		InsecureSkipVerify: false,
		Timeout:            10 * time.Second,
		KeepAlive:          true,
		MaxRetries:         3,
	}, sessionRegistry)
	if err != nil {
		return
	}
	defer func(grpcClient *client.Client) {
		err := grpcClient.Close()
		if err != nil {

		}
	}(grpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	err = grpcClient.CheckServerHealth(ctx)
	if err != nil {
		log.Fatalf("gRPC health check failed: %s", err)
		return
	}
	cancel()

	ctx, cancel = context.WithCancel(context.Background())
	//go func(err error) {
	//	if !errors.Is(err, ctx.Err()) {
	//		log.Fatalf("Event subscription error: %s", err)
	//	}
	//}(grpcClient.SubscribeEvents(ctx))
	go func() {
		err := grpcClient.SubscribeEvents(ctx)
		if err != nil {
			return
		}
	}()

	app, err := server.NewServer(sshConfig, sessionRegistry, grpcClient)
	if err != nil {
		log.Fatalf("Failed to start server: %s", err)
	}
	app.Start()
	cancel()
}
