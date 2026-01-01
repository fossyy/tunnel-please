package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"time"
	"tunnel_pls/session"

	"git.fossy.my.id/bagas/tunnel-please-grpc/gen"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

type GrpcConfig struct {
	Address            string
	UseTLS             bool
	InsecureSkipVerify bool
	Timeout            time.Duration
	KeepAlive          bool
	MaxRetries         int
}

type Client struct {
	conn            *grpc.ClientConn
	config          *GrpcConfig
	sessionRegistry session.Registry
	IdentityService gen.IdentityClient
	eventService    gen.EventServiceClient
}

func DefaultConfig() *GrpcConfig {
	return &GrpcConfig{
		Address:            "localhost:50051",
		UseTLS:             false,
		InsecureSkipVerify: false,
		Timeout:            10 * time.Second,
		KeepAlive:          true,
		MaxRetries:         3,
	}
}

func New(config *GrpcConfig, sessionRegistry session.Registry) (*Client, error) {
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
			PermitWithoutStream: false,
		}
		opts = append(opts, grpc.WithKeepaliveParams(kaParams))
	}

	opts = append(opts,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(4*1024*1024),
			grpc.MaxCallSendMsgSize(4*1024*1024),
		),
	)

	conn, err := grpc.NewClient(config.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server at %s: %w", config.Address, err)
	}

	identityService := gen.NewIdentityClient(conn)
	eventService := gen.NewEventServiceClient(conn)

	return &Client{
		conn:            conn,
		config:          config,
		IdentityService: identityService,
		eventService:    eventService,
		sessionRegistry: sessionRegistry,
	}, nil
}

func (c *Client) SubscribeEvents(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			log.Println("Context cancelled, stopping event subscription")
			return ctx.Err()
		}

		log.Println("Subscribing to events...")
		stream, err := c.eventService.Subscribe(ctx, &empty.Empty{})
		if err != nil {
			log.Printf("Failed to subscribe: %v. Retrying in 10 seconds...", err)
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		if err := c.processEventStream(ctx, stream); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("Stream error: %v. Reconnecting in 10 seconds...", err)
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (c *Client) processEventStream(ctx context.Context, stream gen.EventService_SubscribeClient) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		event, err := stream.Recv()
		if err != nil {
			st, ok := status.FromError(err)
			if !ok {
				return fmt.Errorf("non-gRPC error: %w", err)
			}

			switch st.Code() {
			case codes.Unavailable, codes.Canceled, codes.DeadlineExceeded:
				return fmt.Errorf("stream closed [%s]: %s", st.Code(), st.Message())
			default:
				return fmt.Errorf("gRPC error [%s]: %s", st.Code(), st.Message())
			}
		}

		if event != nil {
			dataEvent := event.GetDataEvent()
			if dataEvent != nil {
				oldSlug := dataEvent.GetOld()
				newSlug := dataEvent.GetNew()

				userSession, exist := c.sessionRegistry.Get(oldSlug)
				if !exist {
					log.Printf("Session with slug '%s' not found, ignoring event", oldSlug)
					continue
				}
				success := c.sessionRegistry.Update(oldSlug, newSlug)

				if success {
					log.Printf("Successfully updated session slug from '%s' to '%s'", oldSlug, newSlug)
					userSession.GetInteraction().Redraw()
				} else {
					log.Printf("Failed to update session slug from '%s' to '%s'", oldSlug, newSlug)
				}
			}
		}
	}
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

func (c *Client) CheckServerHealth(ctx context.Context) error {
	healthClient := grpc_health_v1.NewHealthClient(c.GetConnection())
	resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "",
	})

	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("server not serving: %v", resp.Status)
	}

	return nil
}

func (c *Client) GetConfig() *GrpcConfig {
	return c.config
}
