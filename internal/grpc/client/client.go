package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"git.fossy.my.id/bagas/tunnel-please-grpc/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type ClientConfig struct {
	Address            string
	UseTLS             bool
	InsecureSkipVerify bool
	Timeout            time.Duration
	KeepAlive          bool
	MaxRetries         int
}

type Client struct {
	conn            *grpc.ClientConn
	config          *ClientConfig
	IdentityService gen.IdentityClient
}

func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		Address:            "localhost:50051",
		UseTLS:             false,
		InsecureSkipVerify: false,
		Timeout:            10 * time.Second,
		KeepAlive:          true,
		MaxRetries:         3,
	}
}

func NewClient(config *ClientConfig) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	var opts []grpc.DialOption

	if config.UseTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify,
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if config.KeepAlive {
		kaParams := keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}
		opts = append(opts, grpc.WithKeepaliveParams(kaParams))
	}

	opts = append(opts,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(4*1024*1024), // 4MB
			grpc.MaxCallSendMsgSize(4*1024*1024), // 4MB
		),
	)

	conn, err := grpc.NewClient(config.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server at %s: %w", config.Address, err)
	}

	log.Printf("Successfully connected to gRPC server at %s", config.Address)
	identityService := gen.NewIdentityClient(conn)
	return &Client{
		conn:            conn,
		config:          config,
		IdentityService: identityService,
	}, nil
}

func (c *Client) GetConnection() *grpc.ClientConn {
	return c.conn
}

func (c *Client) Close() error {
	if c.conn != nil {
		log.Printf("Closing gRPC connection to %s", c.config.Address)
		return c.conn.Close()
	}
	return nil
}

func (c *Client) IsConnected() bool {
	if c.conn == nil {
		return false
	}
	state := c.conn.GetState()
	return state.String() == "READY" || state.String() == "IDLE"
}

func (c *Client) Reconnect() error {
	if err := c.Close(); err != nil {
		log.Printf("Warning: error closing existing connection: %v", err)
	}

	var opts []grpc.DialOption

	if c.config.UseTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: c.config.InsecureSkipVerify,
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if c.config.KeepAlive {
		kaParams := keepalive.ClientParameters{
			Time:                10 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}
		opts = append(opts, grpc.WithKeepaliveParams(kaParams))
	}

	opts = append(opts,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(4*1024*1024),
			grpc.MaxCallSendMsgSize(4*1024*1024),
		),
	)

	conn, err := grpc.NewClient(c.config.Address, opts...)
	if err != nil {
		return fmt.Errorf("failed to reconnect to gRPC server at %s: %w", c.config.Address, err)
	}

	c.conn = conn
	log.Printf("Successfully reconnected to gRPC server at %s", c.config.Address)
	return nil
}

func (c *Client) WaitForReady(ctx context.Context) error {
	if c.conn == nil {
		return fmt.Errorf("connection is nil")
	}

	_, ok := ctx.Deadline()
	if !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	currentState := c.conn.GetState()
	for currentState.String() != "READY" {
		if !c.conn.WaitForStateChange(ctx, currentState) {
			return fmt.Errorf("timeout waiting for connection to be ready")
		}
		currentState = c.conn.GetState()

		if currentState.String() == "READY" || currentState.String() == "IDLE" {
			return nil
		}

		if currentState.String() == "SHUTDOWN" || currentState.String() == "TRANSIENT_FAILURE" {
			return fmt.Errorf("connection is in %s state", currentState.String())
		}
	}

	return nil
}

func (c *Client) GetConfig() *ClientConfig {
	return c.config
}
