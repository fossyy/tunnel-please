package bootstrap

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/grpc/client"
	"tunnel_pls/internal/key"
	"tunnel_pls/internal/port"
	"tunnel_pls/internal/random"
	"tunnel_pls/internal/registry"
	"tunnel_pls/internal/transport"
	"tunnel_pls/internal/version"
	"tunnel_pls/server"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type Bootstrap struct {
	Randomizer      random.Random
	Config          config.Config
	SessionRegistry registry.Registry
	Port            port.Port
}

func New() (*Bootstrap, error) {
	conf, err := config.MustLoad()
	if err != nil {
		return nil, err
	}

	randomizer := random.New()
	sessionRegistry := registry.NewRegistry()

	portManager := port.New()
	if err = portManager.AddRange(conf.AllowedPortsStart(), conf.AllowedPortsEnd()); err != nil {
		return nil, err
	}

	return &Bootstrap{
		Randomizer:      randomizer,
		Config:          conf,
		SessionRegistry: sessionRegistry,
		Port:            portManager,
	}, nil
}

func newSSHConfig(sshKeyPath string) (*ssh.ServerConfig, error) {
	sshCfg := &ssh.ServerConfig{
		NoClientAuth:  true,
		ServerVersion: fmt.Sprintf("SSH-2.0-TunnelPlease-%s", version.GetShortVersion()),
	}

	if err := key.GenerateSSHKeyIfNotExist(sshKeyPath); err != nil {
		return nil, fmt.Errorf("generate ssh key: %w", err)
	}
	privateBytes, err := os.ReadFile(sshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	sshCfg.AddHostKey(private)
	return sshCfg, nil
}

func startGRPCClient(ctx context.Context, conf config.Config, registry registry.Registry, errChan chan<- error) (client.Client, error) {
	grpcAddr := fmt.Sprintf("%s:%s", conf.GRPCAddress(), conf.GRPCPort())
	grpcClient, err := client.New(conf, grpcAddr, registry)
	if err != nil {
		return nil, err
	}
	healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
	defer healthCancel()
	if err = grpcClient.CheckServerHealth(healthCtx); err != nil {
		return nil, fmt.Errorf("gRPC health check failed: %w", err)
	}

	go func() {
		if err = grpcClient.SubscribeEvents(ctx, conf.Domain(), conf.NodeToken()); err != nil {
			errChan <- fmt.Errorf("failed to subscribe to events: %w", err)
		}
	}()

	return grpcClient, nil
}

func startHTTPServer(conf config.Config, registry registry.Registry, errChan chan<- error) {
	httpserver := transport.NewHTTPServer(conf.Domain(), conf.HTTPPort(), registry, conf.TLSRedirect())
	ln, err := httpserver.Listen()
	if err != nil {
		errChan <- fmt.Errorf("failed to start http server: %w", err)
		return
	}
	if err = httpserver.Serve(ln); err != nil {
		errChan <- fmt.Errorf("error when serving http server: %w", err)
	}
}

func startHTTPSServer(conf config.Config, registry registry.Registry, errChan chan<- error) {
	tlsCfg, err := transport.NewTLSConfig(conf)
	if err != nil {
		errChan <- fmt.Errorf("failed to create TLS config: %w", err)
		return
	}
	httpsServer := transport.NewHTTPSServer(conf.Domain(), conf.HTTPSPort(), registry, conf.TLSRedirect(), tlsCfg)
	ln, err := httpsServer.Listen()
	if err != nil {
		errChan <- fmt.Errorf("failed to start https server: %w", err)
		return
	}
	if err = httpsServer.Serve(ln); err != nil {
		errChan <- fmt.Errorf("error when serving https server: %w", err)
	}
}

func startSSHServer(rand random.Random, conf config.Config, sshCfg *ssh.ServerConfig, registry registry.Registry, grpcClient client.Client, portManager port.Port, sshPort string) error {
	sshServer, err := server.New(rand, conf, sshCfg, registry, grpcClient, portManager, sshPort)
	if err != nil {
		return err
	}

	sshServer.Start()

	return sshServer.Close()
}

func startPprof(pprofPort string) {
	pprofAddr := fmt.Sprintf("localhost:%s", pprofPort)
	log.Printf("Starting pprof server on http://%s/debug/pprof/", pprofAddr)
	if err := http.ListenAndServe(pprofAddr, nil); err != nil {
		log.Printf("pprof server error: %v", err)
	}
}

func (b *Bootstrap) Run() error {
	sshConfig, err := newSSHConfig(b.Config.KeyLoc())
	if err != nil {
		return fmt.Errorf("failed to create SSH config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 5)
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	var grpcClient client.Client
	if b.Config.Mode() == types.ServerModeNODE {
		grpcClient, err = startGRPCClient(ctx, b.Config, b.SessionRegistry, errChan)
		if err != nil {
			return fmt.Errorf("failed to start gRPC client: %w", err)
		}
		defer func(grpcClient client.Client) {
			err = grpcClient.Close()
			if err != nil {
				log.Printf("failed to close gRPC client")
			}
		}(grpcClient)
	}

	go startHTTPServer(b.Config, b.SessionRegistry, errChan)

	if b.Config.TLSEnabled() {
		go startHTTPSServer(b.Config, b.SessionRegistry, errChan)
	}

	go func() {
		if err = startSSHServer(b.Randomizer, b.Config, sshConfig, b.SessionRegistry, grpcClient, b.Port, b.Config.SSHPort()); err != nil {
			errChan <- fmt.Errorf("SSH server error: %w", err)
		}
	}()

	if b.Config.PprofEnabled() {
		go startPprof(b.Config.PprofPort())
	}

	log.Println("All services started successfully")

	select {
	case err = <-errChan:
		return fmt.Errorf("service error: %w", err)
	case sig := <-shutdownChan:
		log.Printf("Received signal %s, initiating graceful shutdown", sig)
		cancel()
		return nil
	}
}
