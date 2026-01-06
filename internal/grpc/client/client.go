package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/types"

	"tunnel_pls/session"

	proto "git.fossy.my.id/bagas/tunnel-please-grpc/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	conn                       *grpc.ClientConn
	config                     *GrpcConfig
	sessionRegistry            session.Registry
	eventService               proto.EventServiceClient
	authorizeConnectionService proto.UserServiceClient
	closing                    bool
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

	eventService := proto.NewEventServiceClient(conn)
	authorizeConnectionService := proto.NewUserServiceClient(conn)

	return &Client{
		conn:                       conn,
		config:                     config,
		sessionRegistry:            sessionRegistry,
		eventService:               eventService,
		authorizeConnectionService: authorizeConnectionService,
	}, nil
}

func (c *Client) SubscribeEvents(ctx context.Context, identity, authToken string) error {
	const (
		baseBackoff = time.Second
		maxBackoff  = 30 * time.Second
	)

	backoff := baseBackoff
	wait := func() error {
		if backoff <= 0 {
			return nil
		}
		select {
		case <-time.After(backoff):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	growBackoff := func() {
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	for {
		subscribe, err := c.eventService.Subscribe(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled || ctx.Err() != nil {
				return err
			}
			if !c.isConnectionError(err) || status.Code(err) == codes.Unauthenticated {
				return err
			}
			if err = wait(); err != nil {
				return err
			}
			growBackoff()
			log.Printf("Reconnect to controller within %v sec", backoff.Seconds())
			continue
		}

		err = subscribe.Send(&proto.Node{
			Type: proto.EventType_AUTHENTICATION,
			Payload: &proto.Node_AuthEvent{
				AuthEvent: &proto.Authentication{
					Identity:  identity,
					AuthToken: authToken,
				},
			},
		})

		if err != nil {
			log.Println("Authentication failed to send to gRPC server:", err)
			if c.isConnectionError(err) {
				if err = wait(); err != nil {
					return err
				}
				growBackoff()
				continue
			}
			return err
		}
		log.Println("Authentication Successfully sent to gRPC server")
		backoff = baseBackoff

		if err = c.processEventStream(subscribe); err != nil {
			if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled || ctx.Err() != nil {
				return err
			}
			if c.isConnectionError(err) {
				log.Printf("Reconnect to controller within %v sec", backoff.Seconds())
				if err = wait(); err != nil {
					return err
				}
				growBackoff()
				continue
			}
			return err
		}
	}
}

func (c *Client) processEventStream(subscribe grpc.BidiStreamingClient[proto.Node, proto.Events]) error {
	for {
		recv, err := subscribe.Recv()
		if err != nil {
			return err
		}
		switch recv.GetType() {
		case proto.EventType_SLUG_CHANGE:
			user := recv.GetSlugEvent().GetUser()
			oldSlug := recv.GetSlugEvent().GetOld()
			newSlug := recv.GetSlugEvent().GetNew()
			var userSession *session.SSHSession
			userSession, err = c.sessionRegistry.Get(types.SessionKey{
				Id:   oldSlug,
				Type: types.HTTP,
			})
			if err != nil {
				errSend := subscribe.Send(&proto.Node{
					Type: proto.EventType_SLUG_CHANGE_RESPONSE,
					Payload: &proto.Node_SlugEventResponse{
						SlugEventResponse: &proto.SlugChangeEventResponse{
							Success: false,
							Message: err.Error(),
						},
					},
				})
				if errSend != nil {
					if c.isConnectionError(errSend) {
						return errSend
					}
					log.Printf("non-connection send error for slug change failure: %v", errSend)
				}
				continue
			}
			err = c.sessionRegistry.Update(user, types.SessionKey{
				Id:   oldSlug,
				Type: types.HTTP,
			}, types.SessionKey{
				Id:   newSlug,
				Type: types.HTTP,
			})
			if err != nil {
				errSend := subscribe.Send(&proto.Node{
					Type: proto.EventType_SLUG_CHANGE_RESPONSE,
					Payload: &proto.Node_SlugEventResponse{
						SlugEventResponse: &proto.SlugChangeEventResponse{
							Success: false,
							Message: err.Error(),
						},
					},
				})
				if errSend != nil {
					if c.isConnectionError(errSend) {
						return errSend
					}
					log.Printf("non-connection send error for slug change failure: %v", errSend)
				}
				continue
			}
			userSession.GetInteraction().Redraw()
			err = subscribe.Send(&proto.Node{
				Type: proto.EventType_SLUG_CHANGE_RESPONSE,
				Payload: &proto.Node_SlugEventResponse{
					SlugEventResponse: &proto.SlugChangeEventResponse{
						Success: true,
						Message: "",
					},
				},
			})
			if err != nil {
				if c.isConnectionError(err) {
					log.Printf("connection error sending slug change success: %v", err)
					return err
				}
				log.Printf("non-connection send error for slug change success: %v", err)
				continue
			}
		case proto.EventType_GET_SESSIONS:
			sessions := c.sessionRegistry.GetAllSessionFromUser(recv.GetGetSessionsEvent().GetIdentity())
			var details []*proto.Detail
			for _, ses := range sessions {
				detail := ses.Detail()
				details = append(details, &proto.Detail{
					Node:           config.Getenv("DOMAIN", "localhost"),
					ForwardingType: detail.ForwardingType,
					Slug:           detail.Slug,
					UserId:         detail.UserID,
					Active:         detail.Active,
					StartedAt:      timestamppb.New(detail.StartedAt),
				})
			}
			err = subscribe.Send(&proto.Node{
				Type: proto.EventType_GET_SESSIONS,
				Payload: &proto.Node_GetSessionsEvent{
					GetSessionsEvent: &proto.GetSessionsResponse{
						Details: details,
					},
				},
			})
			if err != nil {
				if c.isConnectionError(err) {
					log.Printf("connection error sending sessions success: %v", err)
					return err
				}
				log.Printf("non-connection send error for sessions success: %v", err)
				continue
			}
		case proto.EventType_TERMINATE_SESSION:
			user := recv.GetTerminateSessionEvent().GetUser()
			tunnelTypeRaw := recv.GetTerminateSessionEvent().GetTunnelType()
			slug := recv.GetTerminateSessionEvent().GetSlug()

			var userSession *session.SSHSession
			var tunnelType types.TunnelType
			if tunnelTypeRaw == proto.TunnelType_HTTP {
				tunnelType = types.HTTP
			} else if tunnelTypeRaw == proto.TunnelType_TCP {
				tunnelType = types.TCP
			} else {
				err = subscribe.Send(&proto.Node{
					Type: proto.EventType_TERMINATE_SESSION,
					Payload: &proto.Node_TerminateSessionEventResponse{
						TerminateSessionEventResponse: &proto.TerminateSessionEventResponse{
							Success: false,
							Message: "unknown tunnel type recived",
						},
					},
				})
				if err != nil {
					if c.isConnectionError(err) {
						log.Printf("connection error sending sessions success: %v", err)
						return err
					}
					log.Printf("non-connection send error for sessions success: %v", err)
				}
				continue
			}
			userSession, err = c.sessionRegistry.GetWithUser(user, types.SessionKey{
				Id:   slug,
				Type: tunnelType,
			})
			if err != nil {
				err = subscribe.Send(&proto.Node{
					Type: proto.EventType_TERMINATE_SESSION,
					Payload: &proto.Node_TerminateSessionEventResponse{
						TerminateSessionEventResponse: &proto.TerminateSessionEventResponse{
							Success: false,
							Message: err.Error(),
						},
					},
				})
				if err != nil {
					if c.isConnectionError(err) {
						log.Printf("connection error sending sessions success: %v", err)
						return err
					}
					log.Printf("non-connection send error for sessions success: %v", err)
				}
				continue
			}
			err = userSession.GetLifecycle().Close()
			if err != nil {
				err = subscribe.Send(&proto.Node{
					Type: proto.EventType_TERMINATE_SESSION,
					Payload: &proto.Node_TerminateSessionEventResponse{
						TerminateSessionEventResponse: &proto.TerminateSessionEventResponse{
							Success: false,
							Message: err.Error(),
						},
					},
				})
				if err != nil {
					if c.isConnectionError(err) {
						log.Printf("connection error sending sessions success: %v", err)
						return err
					}
					log.Printf("non-connection send error for sessions success: %v", err)
				}
				continue
			}
			err = subscribe.Send(&proto.Node{
				Type: proto.EventType_TERMINATE_SESSION,
				Payload: &proto.Node_TerminateSessionEventResponse{
					TerminateSessionEventResponse: &proto.TerminateSessionEventResponse{
						Success: true,
						Message: "",
					},
				},
			})
			if err != nil {
				if c.isConnectionError(err) {
					log.Printf("connection error sending sessions success: %v", err)
					return err
				}
				log.Printf("non-connection send error for sessions success: %v", err)
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

func (c *Client) AuthorizeConn(ctx context.Context, token string) (authorized bool, user string, err error) {
	check, err := c.authorizeConnectionService.Check(ctx, &proto.CheckRequest{AuthToken: token})
	if err != nil {
		return false, "UNAUTHORIZED", err
	}

	if check.GetResponse() == proto.AuthorizationResponse_MESSAGE_TYPE_UNAUTHORIZED {
		return false, "UNAUTHORIZED", nil
	}
	return true, check.GetUser(), nil
}

func (c *Client) Close() error {
	if c.conn != nil {
		log.Printf("Closing gRPC connection to %s", c.config.Address)
		c.closing = true
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

func (c *Client) isConnectionError(err error) bool {
	if c.closing {
		return false
	}
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
