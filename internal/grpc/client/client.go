package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"time"
	"tunnel_pls/session"

	proto "git.fossy.my.id/bagas/tunnel-please-grpc/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

type GrpcConfig struct {
	Address             string
	UseTLS              bool
	InsecureSkipVerify  bool
	Timeout             time.Duration
	KeepAlive           bool
	MaxRetries          int
	KeepAliveTime       time.Duration
	KeepAliveTimeout    time.Duration
	PermitWithoutStream bool
}

type Client struct {
	conn            *grpc.ClientConn
	config          *GrpcConfig
	sessionRegistry session.Registry
	slugService     proto.SlugChangeClient
	eventService    proto.EventServiceClient
}

func DefaultConfig() *GrpcConfig {
	return &GrpcConfig{
		Address:             "localhost:50051",
		UseTLS:              false,
		InsecureSkipVerify:  false,
		Timeout:             10 * time.Second,
		KeepAlive:           true,
		MaxRetries:          3,
		KeepAliveTime:       2 * time.Minute,
		KeepAliveTimeout:    10 * time.Second,
		PermitWithoutStream: false,
	}
}

func New(config *GrpcConfig, sessionRegistry session.Registry) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	} else {
		defaults := DefaultConfig()
		if config.Address == "" {
			config.Address = defaults.Address
		}
		if config.Timeout == 0 {
			config.Timeout = defaults.Timeout
		}
		if config.MaxRetries == 0 {
			config.MaxRetries = defaults.MaxRetries
		}
		if config.KeepAliveTime == 0 {
			config.KeepAliveTime = defaults.KeepAliveTime
		}
		if config.KeepAliveTimeout == 0 {
			config.KeepAliveTimeout = defaults.KeepAliveTimeout
		}
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
			Time:                config.KeepAliveTime,
			Timeout:             config.KeepAliveTimeout,
			PermitWithoutStream: config.PermitWithoutStream,
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

	slugService := proto.NewSlugChangeClient(conn)
	eventService := proto.NewEventServiceClient(conn)

	return &Client{
		conn:            conn,
		config:          config,
		slugService:     slugService,
		sessionRegistry: sessionRegistry,
		eventService:    eventService,
	}, nil
}

func (c *Client) SubscribeEvents(ctx context.Context, identity string) error {
	subscribe, err := c.eventService.Subscribe(ctx)
	if err != nil {
		return err
	}
	err = subscribe.Send(&proto.Client{
		Type: proto.EventType_AUTHENTICATION,
		Payload: &proto.Client_AuthEvent{
			AuthEvent: &proto.Authentication{
				Identity:  identity,
				AuthToken: "test_auth_key",
			},
		},
	})

	if err != nil {
		log.Println("Authentication failed to send to gRPC server:", err)
		return err
	}
	log.Println("Authentication Successfully sent to gRPC server")
	err = c.processEventStream(subscribe)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) processEventStream(subscribe grpc.BidiStreamingClient[proto.Client, proto.Controller]) error {
	for {
		recv, err := subscribe.Recv()
		if err != nil {
			if isConnectionError(err) {
				log.Printf("connection error receiving from gRPC server: %v", err)
				return err
			}
			log.Printf("non-connection receive error from gRPC server: %v", err)
			continue
		}
		switch recv.GetType() {
		case proto.EventType_SLUG_CHANGE:
			oldSlug := recv.GetSlugEvent().GetOld()
			newSlug := recv.GetSlugEvent().GetNew()
			session, err := c.sessionRegistry.Get(oldSlug)
			if err != nil {
				errSend := subscribe.Send(&proto.Client{
					Type: proto.EventType_SLUG_CHANGE_RESPONSE,
					Payload: &proto.Client_SlugEventResponse{
						SlugEventResponse: &proto.SlugChangeEventResponse{
							Success: false,
							Message: err.Error(),
						},
					},
				})
				if errSend != nil {
					if isConnectionError(errSend) {
						log.Printf("connection error sending slug change failure: %v", errSend)
						return errSend
					}
					log.Printf("non-connection send error for slug change failure: %v", errSend)
				}
				continue
			}
			err = c.sessionRegistry.Update(oldSlug, newSlug)
			if err != nil {
				errSend := subscribe.Send(&proto.Client{
					Type: proto.EventType_SLUG_CHANGE_RESPONSE,
					Payload: &proto.Client_SlugEventResponse{
						SlugEventResponse: &proto.SlugChangeEventResponse{
							Success: false,
							Message: err.Error(),
						},
					},
				})
				if errSend != nil {
					if isConnectionError(errSend) {
						log.Printf("connection error sending slug change failure: %v", errSend)
						return errSend
					}
					log.Printf("non-connection send error for slug change failure: %v", errSend)
				}
				continue
			}
			session.GetInteraction().Redraw()
			err = subscribe.Send(&proto.Client{
				Type: proto.EventType_SLUG_CHANGE_RESPONSE,
				Payload: &proto.Client_SlugEventResponse{
					SlugEventResponse: &proto.SlugChangeEventResponse{
						Success: true,
						Message: "",
					},
				},
			})
			if err != nil {
				if isConnectionError(err) {
					log.Printf("connection error sending slug change success: %v", err)
					return err
				}
				log.Printf("non-connection send error for slug change success: %v", err)
				continue
			}
		default:
			log.Printf("Unknown event type received: %v", recv.GetType())
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

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.Canceled, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}
