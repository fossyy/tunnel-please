package client

import (
	"context"
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
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Client interface {
	SubscribeEvents(ctx context.Context, identity, authToken string) error
	ClientConn() *grpc.ClientConn
	AuthorizeConn(ctx context.Context, token string) (authorized bool, user string, err error)
	Close() error
	CheckServerHealth(ctx context.Context) error
}
type client struct {
	conn                       *grpc.ClientConn
	address                    string
	sessionRegistry            session.Registry
	eventService               proto.EventServiceClient
	authorizeConnectionService proto.UserServiceClient
	closing                    bool
}

func New(address string, sessionRegistry session.Registry) (Client, error) {
	var opts []grpc.DialOption

	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	kaParams := keepalive.ClientParameters{
		Time:                2 * time.Minute,
		Timeout:             10 * time.Second,
		PermitWithoutStream: false,
	}

	opts = append(opts, grpc.WithKeepaliveParams(kaParams))

	opts = append(opts,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(4*1024*1024),
			grpc.MaxCallSendMsgSize(4*1024*1024),
		),
	)

	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server at %s: %w", address, err)
	}

	eventService := proto.NewEventServiceClient(conn)
	authorizeConnectionService := proto.NewUserServiceClient(conn)

	return &client{
		conn:                       conn,
		address:                    address,
		sessionRegistry:            sessionRegistry,
		eventService:               eventService,
		authorizeConnectionService: authorizeConnectionService,
	}, nil
}

func (c *client) SubscribeEvents(ctx context.Context, identity, authToken string) error {
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

func (c *client) processEventStream(subscribe grpc.BidiStreamingClient[proto.Node, proto.Events]) error {
	handlers := c.eventHandlers(subscribe)

	for {
		recv, err := subscribe.Recv()
		if err != nil {
			return err
		}

		handler, ok := handlers[recv.GetType()]
		if !ok {
			log.Printf("Unknown event type received: %v", recv.GetType())
			continue
		}

		if err = handler(recv); err != nil {
			return err
		}
	}
}

func (c *client) eventHandlers(subscribe grpc.BidiStreamingClient[proto.Node, proto.Events]) map[proto.EventType]func(*proto.Events) error {
	return map[proto.EventType]func(*proto.Events) error{
		proto.EventType_SLUG_CHANGE:       func(evt *proto.Events) error { return c.handleSlugChange(subscribe, evt) },
		proto.EventType_GET_SESSIONS:      func(evt *proto.Events) error { return c.handleGetSessions(subscribe, evt) },
		proto.EventType_TERMINATE_SESSION: func(evt *proto.Events) error { return c.handleTerminateSession(subscribe, evt) },
	}
}

func (c *client) handleSlugChange(subscribe grpc.BidiStreamingClient[proto.Node, proto.Events], evt *proto.Events) error {
	slugEvent := evt.GetSlugEvent()
	user := slugEvent.GetUser()
	oldSlug := slugEvent.GetOld()
	newSlug := slugEvent.GetNew()

	userSession, err := c.sessionRegistry.Get(types.SessionKey{Id: oldSlug, Type: types.HTTP})
	if err != nil {
		return c.sendNode(subscribe, &proto.Node{
			Type: proto.EventType_SLUG_CHANGE_RESPONSE,
			Payload: &proto.Node_SlugEventResponse{
				SlugEventResponse: &proto.SlugChangeEventResponse{Success: false, Message: err.Error()},
			},
		}, "slug change failure response")
	}

	if err = c.sessionRegistry.Update(user, types.SessionKey{Id: oldSlug, Type: types.HTTP}, types.SessionKey{Id: newSlug, Type: types.HTTP}); err != nil {
		return c.sendNode(subscribe, &proto.Node{
			Type: proto.EventType_SLUG_CHANGE_RESPONSE,
			Payload: &proto.Node_SlugEventResponse{
				SlugEventResponse: &proto.SlugChangeEventResponse{Success: false, Message: err.Error()},
			},
		}, "slug change failure response")
	}

	userSession.Interaction().Redraw()
	return c.sendNode(subscribe, &proto.Node{
		Type: proto.EventType_SLUG_CHANGE_RESPONSE,
		Payload: &proto.Node_SlugEventResponse{
			SlugEventResponse: &proto.SlugChangeEventResponse{Success: true, Message: ""},
		},
	}, "slug change success response")
}

func (c *client) handleGetSessions(subscribe grpc.BidiStreamingClient[proto.Node, proto.Events], evt *proto.Events) error {
	sessions := c.sessionRegistry.GetAllSessionFromUser(evt.GetGetSessionsEvent().GetIdentity())

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

	return c.sendNode(subscribe, &proto.Node{
		Type: proto.EventType_GET_SESSIONS,
		Payload: &proto.Node_GetSessionsEvent{
			GetSessionsEvent: &proto.GetSessionsResponse{Details: details},
		},
	}, "send get sessions response")
}

func (c *client) handleTerminateSession(subscribe grpc.BidiStreamingClient[proto.Node, proto.Events], evt *proto.Events) error {
	terminate := evt.GetTerminateSessionEvent()
	user := terminate.GetUser()
	slug := terminate.GetSlug()

	tunnelType, err := c.protoToTunnelType(terminate.GetTunnelType())
	if err != nil {
		return c.sendNode(subscribe, &proto.Node{
			Type: proto.EventType_TERMINATE_SESSION,
			Payload: &proto.Node_TerminateSessionEventResponse{
				TerminateSessionEventResponse: &proto.TerminateSessionEventResponse{Success: false, Message: err.Error()},
			},
		}, "terminate session invalid tunnel type")
	}

	userSession, err := c.sessionRegistry.GetWithUser(user, types.SessionKey{Id: slug, Type: tunnelType})
	if err != nil {
		return c.sendNode(subscribe, &proto.Node{
			Type: proto.EventType_TERMINATE_SESSION,
			Payload: &proto.Node_TerminateSessionEventResponse{
				TerminateSessionEventResponse: &proto.TerminateSessionEventResponse{Success: false, Message: err.Error()},
			},
		}, "terminate session fetch failed")
	}

	if err = userSession.Lifecycle().Close(); err != nil {
		return c.sendNode(subscribe, &proto.Node{
			Type: proto.EventType_TERMINATE_SESSION,
			Payload: &proto.Node_TerminateSessionEventResponse{
				TerminateSessionEventResponse: &proto.TerminateSessionEventResponse{Success: false, Message: err.Error()},
			},
		}, "terminate session close failed")
	}

	return c.sendNode(subscribe, &proto.Node{
		Type: proto.EventType_TERMINATE_SESSION,
		Payload: &proto.Node_TerminateSessionEventResponse{
			TerminateSessionEventResponse: &proto.TerminateSessionEventResponse{Success: true, Message: ""},
		},
	}, "terminate session success response")
}

func (c *client) sendNode(subscribe grpc.BidiStreamingClient[proto.Node, proto.Events], node *proto.Node, context string) error {
	if err := subscribe.Send(node); err != nil {
		if c.isConnectionError(err) {
			return err
		}
		log.Printf("%s: %v", context, err)
	}
	return nil
}

func (c *client) protoToTunnelType(t proto.TunnelType) (types.TunnelType, error) {
	switch t {
	case proto.TunnelType_HTTP:
		return types.HTTP, nil
	case proto.TunnelType_TCP:
		return types.TCP, nil
	default:
		return types.UNKNOWN, fmt.Errorf("unknown tunnel type received")
	}
}

func (c *client) ClientConn() *grpc.ClientConn {
	return c.conn
}

func (c *client) AuthorizeConn(ctx context.Context, token string) (authorized bool, user string, err error) {
	check, err := c.authorizeConnectionService.Check(ctx, &proto.CheckRequest{AuthToken: token})
	if err != nil {
		return false, "UNAUTHORIZED", err
	}

	if check.GetResponse() == proto.AuthorizationResponse_MESSAGE_TYPE_UNAUTHORIZED {
		return false, "UNAUTHORIZED", nil
	}
	return true, check.GetUser(), nil
}

func (c *client) CheckServerHealth(ctx context.Context) error {
	healthClient := grpc_health_v1.NewHealthClient(c.ClientConn())
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

func (c *client) Close() error {
	if c.conn != nil {
		log.Printf("Closing gRPC connection to %s", c.address)
		c.closing = true
		return c.conn.Close()
	}
	return nil
}

func (c *client) isConnectionError(err error) bool {
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
