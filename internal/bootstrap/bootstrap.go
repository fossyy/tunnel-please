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
	GrpcClient      client.Client
	ErrChan         chan error
	SignalChan      chan os.Signal
}

func New(config config.Config, port port.Port) (*Bootstrap, error) {
	randomizer := random.New()
	sessionRegistry := registry.NewRegistry()

	if err := port.AddRange(config.AllowedPortsStart(), config.AllowedPortsEnd()); err != nil {
		return nil, err
	}

	grpcClient, err := client.New(config, sessionRegistry)
	if err != nil {
		return nil, err
	}

	errChan := make(chan error, 5)
	signalChan := make(chan os.Signal, 1)

	return &Bootstrap{
		Randomizer:      randomizer,
		Config:          config,
		SessionRegistry: sessionRegistry,
		Port:            port,
		GrpcClient:      grpcClient,
		ErrChan:         errChan,
		SignalChan:      signalChan,
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

func (b *Bootstrap) startGRPCClient(ctx context.Context, conf config.Config, errChan chan<- error) error {
	healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
	defer healthCancel()
	if err := b.GrpcClient.CheckServerHealth(healthCtx); err != nil {
		return fmt.Errorf("gRPC health check failed: %w", err)
	}

	go func() {
		if err := b.GrpcClient.SubscribeEvents(ctx, conf.Domain(), conf.NodeToken()); err != nil {
			errChan <- fmt.Errorf("failed to subscribe to events: %w", err)
		}
	}()

	return nil
}

func startHTTPServer(conf config.Config, registry registry.Registry, errChan chan<- error) {
	httpserver := transport.NewHTTPServer(conf, registry)
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
	httpsServer := transport.NewHTTPSServer(conf, registry, tlsCfg)
	ln, err := httpsServer.Listen()
	if err != nil {
		errChan <- fmt.Errorf("failed to create TLS config: %w", err)
		return
	}
	if err = httpsServer.Serve(ln); err != nil {
		errChan <- fmt.Errorf("error when serving https server: %w", err)
	}
}

func startSSHServer(rand random.Random, conf config.Config, sshCfg *ssh.ServerConfig, registry registry.Registry, grpcClient client.Client, portManager port.Port, errChan chan<- error) {
	sshServer, err := server.New(rand, conf, sshCfg, registry, grpcClient, portManager, conf.SSHPort())
	if err != nil {
		errChan <- err
		return
	}

	sshServer.Start()

	errChan <- sshServer.Close()
}

func startPprof(pprofPort string, errChan chan<- error) {
	pprofAddr := fmt.Sprintf("localhost:%s", pprofPort)
	log.Printf("Starting pprof server on http://%s/debug/pprof/", pprofAddr)
	if err := http.ListenAndServe(pprofAddr, nil); err != nil {
		errChan <- fmt.Errorf("pprof server error: %v", err)
	}
}
func (b *Bootstrap) Run() error {
	sshConfig, err := newSSHConfig(b.Config.KeyLoc())
	if err != nil {
		return fmt.Errorf("failed to create SSH config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signal.Notify(b.SignalChan, os.Interrupt, syscall.SIGTERM)

	if b.Config.Mode() == types.ServerModeNODE {
		err = b.startGRPCClient(ctx, b.Config, b.ErrChan)
		if err != nil {
			return fmt.Errorf("failed to start gRPC client: %w", err)
		}
		defer func(grpcClient client.Client) {
			err = grpcClient.Close()
			if err != nil {
				log.Printf("failed to close gRPC client")
			}
		}(b.GrpcClient)
	}

	go startHTTPServer(b.Config, b.SessionRegistry, b.ErrChan)

	if b.Config.TLSEnabled() {
		go startHTTPSServer(b.Config, b.SessionRegistry, b.ErrChan)
	}

	go func() {
		startSSHServer(b.Randomizer, b.Config, sshConfig, b.SessionRegistry, b.GrpcClient, b.Port, b.ErrChan)
	}()

	if b.Config.PprofEnabled() {
		go startPprof(b.Config.PprofPort(), b.ErrChan)
	}

	log.Println("All services started successfully")

	select {
	case err = <-b.ErrChan:
		return fmt.Errorf("service error: %w", err)
	case sig := <-b.SignalChan:
		log.Printf("Received signal %s, initiating graceful shutdown", sig)
		cancel()
		return nil
	}
}
