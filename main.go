package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/grpc/client"
	"tunnel_pls/internal/key"
	"tunnel_pls/internal/port"
	"tunnel_pls/internal/registry"
	"tunnel_pls/internal/transport"
	"tunnel_pls/internal/version"
	"tunnel_pls/server"
	"tunnel_pls/types"

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

	conf, err := config.MustLoad()
	if err != nil {
		log.Fatalf("Failed to load configuration: %s", err)
		return
	}

	sshConfig := &ssh.ServerConfig{
		NoClientAuth:  true,
		ServerVersion: fmt.Sprintf("SSH-2.0-TunnelPlease-%s", version.GetShortVersion()),
	}

	sshKeyPath := "certs/ssh/id_rsa"
	if err = key.GenerateSSHKeyIfNotExist(sshKeyPath); err != nil {
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
	sessionRegistry := registry.NewRegistry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 2)
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	var grpcClient client.Client

	if conf.Mode() == types.ServerModeNODE {
		grpcAddr := fmt.Sprintf("%s:%s", conf.GRPCAddress(), conf.GRPCPort())

		grpcClient, err = client.New(conf, grpcAddr, sessionRegistry)
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
			if err = grpcClient.SubscribeEvents(ctx, conf.Domain(), conf.NodeToken()); err != nil {
				errChan <- fmt.Errorf("failed to subscribe to events: %w", err)
			}
		}()
	}

	go func() {
		var httpListener net.Listener
		httpserver := transport.NewHTTPServer(conf.Domain(), conf.HTTPPort(), sessionRegistry, conf.TLSRedirect())
		httpListener, err = httpserver.Listen()
		if err != nil {
			errChan <- fmt.Errorf("failed to start http server: %w", err)
			return
		}
		err = httpserver.Serve(httpListener)
		if err != nil {
			errChan <- fmt.Errorf("error when serving http server: %w", err)
			return
		}
	}()

	if conf.TLSEnabled() {
		go func() {
			var httpsListener net.Listener
			tlsConfig, _ := transport.NewTLSConfig(conf)
			httpsServer := transport.NewHTTPSServer(conf.Domain(), conf.HTTPSPort(), sessionRegistry, conf.TLSRedirect(), tlsConfig)
			httpsListener, err = httpsServer.Listen()
			if err != nil {
				errChan <- fmt.Errorf("failed to start http server: %w", err)
				return
			}
			err = httpsServer.Serve(httpsListener)
			if err != nil {
				errChan <- fmt.Errorf("error when serving http server: %w", err)
				return
			}
		}()
	}
	
	portManager := port.New()
	err = portManager.AddRange(conf.AllowedPortsStart(), conf.AllowedPortsEnd())
	if err != nil {
		log.Fatalf("Failed to initialize port manager: %s", err)
		return
	}

	var app server.Server
	go func() {
		app, err = server.New(conf, sshConfig, sessionRegistry, grpcClient, portManager, conf.SSHPort())
		if err != nil {
			errChan <- fmt.Errorf("failed to start server: %s", err)
			return
		}
		app.Start()

	}()

	if conf.PprofEnabled() {
		go func() {
			pprofAddr := fmt.Sprintf("localhost:%s", conf.PprofPort())
			log.Printf("Starting pprof server on http://%s/debug/pprof/", pprofAddr)
			if err = http.ListenAndServe(pprofAddr, nil); err != nil {
				log.Printf("pprof server error: %v", err)
			}
		}()
	}

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
