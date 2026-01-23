package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"tunnel_pls/internal/config"
	"tunnel_pls/internal/registry"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	proto "git.fossy.my.id/bagas/tunnel-please-grpc/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestClient_ClientConn(t *testing.T) {
	conn := &grpc.ClientConn{}
	c := &client{conn: conn}
	if c.ClientConn() != conn {
		t.Errorf("ClientConn() did not return expected connection")
	}
}

func TestClient_Close(t *testing.T) {
	c := &client{}
	if err := c.Close(); err != nil {
		t.Errorf("Close() on nil connection returned error: %v", err)
	}
}

func TestAuthorizeConn(t *testing.T) {
	mockUserSvc := &mockUserServiceClient{}
	c := &client{authorizeConnectionService: mockUserSvc}

	tests := []struct {
		name     string
		token    string
		mockResp *proto.CheckResponse
		mockErr  error
		wantAuth bool
		wantUser string
		wantErr  bool
	}{
		{
			name:     "Success",
			token:    "valid",
			mockResp: &proto.CheckResponse{Response: proto.AuthorizationResponse_MESSAGE_TYPE_AUTHORIZED, User: "mas-fuad"},
			wantAuth: true,
			wantUser: "mas-fuad",
			wantErr:  false,
		},
		{
			name:     "Unauthorized",
			token:    "invalid",
			mockResp: &proto.CheckResponse{Response: proto.AuthorizationResponse_MESSAGE_TYPE_UNAUTHORIZED},
			wantAuth: false,
			wantUser: "UNAUTHORIZED",
			wantErr:  false,
		},
		{
			name:     "Error",
			token:    "error",
			mockErr:  errors.New("grpc error"),
			wantAuth: false,
			wantUser: "UNAUTHORIZED",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUserSvc.checkFunc = func(ctx context.Context, in *proto.CheckRequest, opts ...grpc.CallOption) (*proto.CheckResponse, error) {
				if in.AuthToken != tt.token {
					t.Errorf("expected token %s, got %s", tt.token, in.AuthToken)
				}
				return tt.mockResp, tt.mockErr
			}

			auth, user, err := c.AuthorizeConn(context.Background(), tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("AuthorizeConn() error = %v, wantErr %v", err, tt.wantErr)
			}
			if auth != tt.wantAuth {
				t.Errorf("AuthorizeConn() auth = %v, wantAuth %v", auth, tt.wantAuth)
			}
			if user != tt.wantUser {
				t.Errorf("AuthorizeConn() user = %s, wantUser %s", user, tt.wantUser)
			}
		})
	}
}

func TestHandleSubscribeError(t *testing.T) {
	c := &client{}
	ctx := context.Background()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		backoff time.Duration
		wantErr bool
		wantB   time.Duration
	}{
		{
			name:    "ContextCanceled",
			ctx:     canceledCtx,
			err:     context.Canceled,
			backoff: time.Second,
			wantErr: true,
		},
		{
			name:    "GrpcCanceled",
			ctx:     ctx,
			err:     status.Error(codes.Canceled, "canceled"),
			backoff: time.Second,
			wantErr: true,
		},
		{
			name:    "CtxErrSet",
			ctx:     canceledCtx,
			err:     errors.New("other error"),
			backoff: time.Second,
			wantErr: true,
		},
		{
			name:    "Unauthenticated",
			ctx:     ctx,
			err:     status.Error(codes.Unauthenticated, "unauth"),
			backoff: time.Second,
			wantErr: true,
		},
		{
			name:    "ConnectionError",
			ctx:     ctx,
			err:     status.Error(codes.Unavailable, "unavailable"),
			backoff: time.Second,
			wantErr: false,
			wantB:   2 * time.Second,
		},
		{
			name:    "NonConnectionError",
			ctx:     ctx,
			err:     status.Error(codes.Internal, "internal"),
			backoff: time.Second,
			wantErr: true,
		},
		{
			name:    "WaitCanceled",
			ctx:     canceledCtx,
			err:     status.Error(codes.Unavailable, "unavailable"),
			backoff: time.Second,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := tt.backoff
			err := c.handleSubscribeError(tt.ctx, tt.err, &backoff)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleSubscribeError() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && backoff != tt.wantB {
				t.Errorf("handleSubscribeError() backoff = %v, want %v", backoff, tt.wantB)
			}
		})
	}

	t.Run("WaitCanceledReal", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		backoff := 50 * time.Millisecond
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		err := c.handleSubscribeError(ctx, status.Error(codes.Unavailable, "unavailable"), &backoff)
		if err == nil {
			t.Errorf("expected error from wait")
		}
	})
}

func TestHandleStreamError(t *testing.T) {
	c := &client{}
	ctx := context.Background()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		backoff time.Duration
		wantErr bool
		wantB   time.Duration
	}{
		{
			name:    "ContextCanceled",
			ctx:     canceledCtx,
			err:     context.Canceled,
			backoff: time.Second,
			wantErr: true,
		},
		{
			name:    "GrpcCanceled",
			ctx:     ctx,
			err:     status.Error(codes.Canceled, "canceled"),
			backoff: time.Second,
			wantErr: true,
		},
		{
			name:    "CtxErrSet",
			ctx:     canceledCtx,
			err:     errors.New("other error"),
			backoff: time.Second,
			wantErr: true,
		},
		{
			name:    "ConnectionError",
			ctx:     ctx,
			err:     status.Error(codes.Unavailable, "unavailable"),
			backoff: time.Second,
			wantErr: false,
			wantB:   2 * time.Second,
		},
		{
			name:    "NonConnectionError",
			ctx:     ctx,
			err:     status.Error(codes.Internal, "internal"),
			backoff: time.Second,
			wantErr: true,
		},
		{
			name:    "WaitCanceled",
			ctx:     canceledCtx,
			err:     status.Error(codes.Unavailable, "unavailable"),
			backoff: time.Second,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := tt.backoff
			err := c.handleStreamError(tt.ctx, tt.err, &backoff)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleStreamError() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && backoff != tt.wantB {
				t.Errorf("handleStreamError() backoff = %v, want %v", backoff, tt.wantB)
			}
		})
	}

	t.Run("WaitCanceledReal", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		backoff := 50 * time.Millisecond
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		err := c.handleStreamError(ctx, status.Error(codes.Unavailable, "unavailable"), &backoff)
		if err == nil {
			t.Errorf("expected error from wait")
		}
	})
}

func TestHandleAuthError(t *testing.T) {
	c := &client{}
	ctx := context.Background()

	tests := []struct {
		name    string
		err     error
		backoff time.Duration
		wantErr bool
		wantB   time.Duration
	}{
		{
			name:    "ConnectionError",
			err:     status.Error(codes.Unavailable, "unavailable"),
			backoff: time.Second,
			wantErr: false,
			wantB:   2 * time.Second,
		},
		{
			name:    "NonConnectionError",
			err:     status.Error(codes.Internal, "internal"),
			backoff: time.Second,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := tt.backoff
			err := c.handleAuthError(ctx, tt.err, &backoff)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleAuthError() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && backoff != tt.wantB {
				t.Errorf("handleAuthError() backoff = %v, want %v", backoff, tt.wantB)
			}
		})
	}
}

func TestHandleAuthError_WaitFail(t *testing.T) {
	c := &client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backoff := time.Second
	err := c.handleAuthError(ctx, status.Error(codes.Unavailable, "unavailable"), &backoff)
	if err == nil {
		t.Errorf("expected error when wait fails")
	}
}

func TestProcessEventStream(t *testing.T) {
	mockStream := &mockSubscribeClient{}
	c := &client{}

	t.Run("UnknownEventType", func(t *testing.T) {
		mockStream.recvFunc = func() (*proto.Events, error) {
			return &proto.Events{Type: proto.EventType(999)}, nil
		}
		first := true
		mockStream.recvFunc = func() (*proto.Events, error) {
			if first {
				first = false
				return &proto.Events{Type: proto.EventType(999)}, nil
			}
			return nil, io.EOF
		}
		err := c.processEventStream(mockStream)
		if !errors.Is(err, io.EOF) {
			t.Errorf("expected EOF, got %v", err)
		}
	})

	t.Run("DispatchSuccess", func(t *testing.T) {
		events := []proto.EventType{
			proto.EventType_SLUG_CHANGE,
			proto.EventType_GET_SESSIONS,
			proto.EventType_TERMINATE_SESSION,
		}

		for _, et := range events {
			t.Run(et.String(), func(t *testing.T) {
				first := true
				mockStream.recvFunc = func() (*proto.Events, error) {
					if first {
						first = false
						payload := &proto.Events{Type: et}
						switch et {
						case proto.EventType_SLUG_CHANGE:
							payload.Payload = &proto.Events_SlugEvent{SlugEvent: &proto.SlugChangeEvent{}}
						case proto.EventType_GET_SESSIONS:
							payload.Payload = &proto.Events_GetSessionsEvent{GetSessionsEvent: &proto.GetSessionsEvent{}}
						case proto.EventType_TERMINATE_SESSION:
							payload.Payload = &proto.Events_TerminateSessionEvent{TerminateSessionEvent: &proto.TerminateSessionEvent{}}
						}
						return payload, nil
					}
					return nil, io.EOF
				}
				mockReg := &mockRegistry{}
				mockReg.getAllSessionFromUserFunc = func(user string) []registry.Session { return nil }
				mockReg.getFunc = func(key registry.Key) (registry.Session, error) { return nil, errors.New("fail") }
				mockReg.getWithUserFunc = func(user string, key registry.Key) (registry.Session, error) { return nil, errors.New("fail") }
				c.sessionRegistry = mockReg
				c.config = &mockConfig{domain: "test.com"}
				mockStream.sendFunc = func(n *proto.Node) error { return nil }

				err := c.processEventStream(mockStream)
				if !errors.Is(err, io.EOF) {
					t.Errorf("expected EOF, got %v", err)
				}
			})
		}
	})

	t.Run("HandlerError", func(t *testing.T) {
		first := true
		mockStream.recvFunc = func() (*proto.Events, error) {
			if first {
				first = false
				return &proto.Events{Type: proto.EventType_SLUG_CHANGE, Payload: &proto.Events_SlugEvent{SlugEvent: &proto.SlugChangeEvent{}}}, nil
			}
			return nil, io.EOF
		}
		mockReg := &mockRegistry{}
		mockReg.getFunc = func(key registry.Key) (registry.Session, error) { return nil, errors.New("fail") }
		c.sessionRegistry = mockReg
		mockStream.sendFunc = func(n *proto.Node) error { return status.Error(codes.Unavailable, "send fail") }

		err := c.processEventStream(mockStream)
		if !errors.Is(err, status.Error(codes.Unavailable, "send fail")) {
			t.Errorf("expected send fail error, got %v", err)
		}
	})
}

func TestSendNode(t *testing.T) {
	c := &client{}
	mockStream := &mockSubscribeClient{}

	t.Run("Success", func(t *testing.T) {
		mockStream.sendFunc = func(n *proto.Node) error { return nil }
		err := c.sendNode(mockStream, &proto.Node{}, "context")
		if err != nil {
			t.Errorf("sendNode error = %v", err)
		}
	})

	t.Run("ConnectionError", func(t *testing.T) {
		mockStream.sendFunc = func(n *proto.Node) error { return status.Error(codes.Unavailable, "fail") }
		err := c.sendNode(mockStream, &proto.Node{}, "context")
		if err == nil {
			t.Errorf("expected error")
		}
	})

	t.Run("OtherError", func(t *testing.T) {
		mockStream.sendFunc = func(n *proto.Node) error { return status.Error(codes.Internal, "fail") }
		err := c.sendNode(mockStream, &proto.Node{}, "context")
		if err != nil {
			t.Errorf("expected nil error for non-connection error (logged only)")
		}
	})
}

func TestHandleSlugChange(t *testing.T) {
	mockReg := &mockRegistry{}
	mockStream := &mockSubscribeClient{}
	c := &client{sessionRegistry: mockReg}

	evt := &proto.Events{
		Payload: &proto.Events_SlugEvent{
			SlugEvent: &proto.SlugChangeEvent{
				User: "mas-fuad",
				Old:  "old-slug",
				New:  "new-slug",
			},
		},
	}

	t.Run("Success", func(t *testing.T) {
		mockSess := &mockSession{}
		mockInter := &mockInteraction{}
		mockSess.interactionFunc = func() interaction.Interaction { return mockInter }

		mockReg.getFunc = func(key registry.Key) (registry.Session, error) {
			if key.Id != "old-slug" {
				t.Errorf("expected old-slug, got %s", key.Id)
			}
			return mockSess, nil
		}
		mockReg.updateFunc = func(user string, oldKey, newKey registry.Key) error {
			if user != "mas-fuad" || oldKey.Id != "old-slug" || newKey.Id != "new-slug" {
				t.Errorf("unexpected update args")
			}
			return nil
		}

		sent := false
		mockStream.sendFunc = func(n *proto.Node) error {
			sent = true
			if n.Type != proto.EventType_SLUG_CHANGE_RESPONSE {
				t.Errorf("expected slug change response")
			}
			resp := n.GetSlugEventResponse()
			if !resp.Success {
				t.Errorf("expected success")
			}
			return nil
		}

		err := c.handleSlugChange(mockStream, evt)
		if err != nil {
			t.Errorf("handleSlugChange error = %v", err)
		}
		if !mockInter.redrawCalled {
			t.Errorf("redraw was not called")
		}
		if !sent {
			t.Errorf("response not sent")
		}
	})

	t.Run("SessionNotFound", func(t *testing.T) {
		mockReg.getFunc = func(key registry.Key) (registry.Session, error) {
			return nil, errors.New("not found")
		}
		mockStream.sendFunc = func(n *proto.Node) error {
			resp := n.GetSlugEventResponse()
			if resp.Success || resp.Message != "not found" {
				t.Errorf("unexpected failure response: %v", resp)
			}
			return nil
		}
		err := c.handleSlugChange(mockStream, evt)
		if err != nil {
			t.Errorf("handleSlugChange should return nil if error is handled via response, but it currently returns whatever sendNode returns")
		}
	})

	t.Run("UpdateError", func(t *testing.T) {
		mockSess := &mockSession{}
		mockReg.getFunc = func(key registry.Key) (registry.Session, error) { return mockSess, nil }
		mockReg.updateFunc = func(user string, oldKey, newKey registry.Key) error {
			return errors.New("update fail")
		}
		mockStream.sendFunc = func(n *proto.Node) error {
			resp := n.GetSlugEventResponse()
			if resp.Success || resp.Message != "update fail" {
				t.Errorf("unexpected failure response: %v", resp)
			}
			return nil
		}
		err := c.handleSlugChange(mockStream, evt)
		if err != nil {
			t.Errorf("handleSlugChange error = %v", err)
		}
	})
}

func TestHandleGetSessions(t *testing.T) {
	mockReg := &mockRegistry{}
	mockStream := &mockSubscribeClient{}
	mockCfg := &mockConfig{domain: "test.com"}
	c := &client{sessionRegistry: mockReg, config: mockCfg}

	evt := &proto.Events{
		Payload: &proto.Events_GetSessionsEvent{
			GetSessionsEvent: &proto.GetSessionsEvent{
				Identity: "mas-fuad",
			},
		},
	}

	t.Run("Success", func(t *testing.T) {
		now := time.Now()
		mockSess := &mockSession{}
		mockSess.detailFunc = func() *types.Detail {
			return &types.Detail{
				ForwardingType: "http",
				Slug:           "myslug",
				UserID:         "mas-fuad",
				Active:         true,
				StartedAt:      now,
			}
		}

		mockReg.getAllSessionFromUserFunc = func(user string) []registry.Session {
			if user != "mas-fuad" {
				t.Errorf("expected mas-fuad, got %s", user)
			}
			return []registry.Session{mockSess}
		}

		sent := false
		mockStream.sendFunc = func(n *proto.Node) error {
			sent = true
			if n.Type != proto.EventType_GET_SESSIONS {
				t.Errorf("expected get sessions response type")
			}
			resp := n.GetGetSessionsEvent()
			if len(resp.Details) != 1 || resp.Details[0].Slug != "myslug" {
				t.Errorf("unexpected details: %v", resp.Details)
			}
			return nil
		}

		err := c.handleGetSessions(mockStream, evt)
		if err != nil {
			t.Errorf("handleGetSessions error = %v", err)
		}
		if !sent {
			t.Errorf("response not sent")
		}
	})
}

func TestHandleTerminateSession(t *testing.T) {
	mockReg := &mockRegistry{}
	mockStream := &mockSubscribeClient{}
	c := &client{sessionRegistry: mockReg}

	evt := &proto.Events{
		Payload: &proto.Events_TerminateSessionEvent{
			TerminateSessionEvent: &proto.TerminateSessionEvent{
				User:       "mas-fuad",
				Slug:       "myslug",
				TunnelType: proto.TunnelType_HTTP,
			},
		},
	}

	t.Run("Success", func(t *testing.T) {
		mockSess := &mockSession{}
		mockLife := &mockLifecycle{}
		mockSess.lifecycleFunc = func() lifecycle.Lifecycle { return mockLife }

		closed := false
		mockLife.closeFunc = func() error {
			closed = true
			return nil
		}

		mockReg.getWithUserFunc = func(user string, key registry.Key) (registry.Session, error) {
			if user != "mas-fuad" || key.Id != "myslug" || key.Type != types.TunnelTypeHTTP {
				t.Errorf("unexpected get args")
			}
			return mockSess, nil
		}

		sent := false
		mockStream.sendFunc = func(n *proto.Node) error {
			sent = true
			resp := n.GetTerminateSessionEventResponse()
			if !resp.Success {
				t.Errorf("expected success")
			}
			return nil
		}

		err := c.handleTerminateSession(mockStream, evt)
		if err != nil {
			t.Errorf("handleTerminateSession error = %v", err)
		}
		if !closed {
			t.Errorf("close was not called")
		}
		if !sent {
			t.Errorf("response not sent")
		}
	})

	t.Run("TunnelTypeUnknown", func(t *testing.T) {
		badEvt := &proto.Events{
			Payload: &proto.Events_TerminateSessionEvent{
				TerminateSessionEvent: &proto.TerminateSessionEvent{
					TunnelType: proto.TunnelType(999),
				},
			},
		}
		mockStream.sendFunc = func(n *proto.Node) error {
			resp := n.GetTerminateSessionEventResponse()
			if resp.Success || resp.Message == "" {
				t.Errorf("expected failure response")
			}
			return nil
		}
		err := c.handleTerminateSession(mockStream, badEvt)
		if err != nil {
			t.Errorf("handleTerminateSession error = %v", err)
		}
	})

	t.Run("SessionNotFound", func(t *testing.T) {
		mockReg.getWithUserFunc = func(user string, key registry.Key) (registry.Session, error) {
			return nil, errors.New("not found")
		}
		mockStream.sendFunc = func(n *proto.Node) error {
			resp := n.GetTerminateSessionEventResponse()
			if resp.Success || resp.Message != "not found" {
				t.Errorf("unexpected failure response: %v", resp)
			}
			return nil
		}
		err := c.handleTerminateSession(mockStream, evt)
		if err != nil {
			t.Errorf("handleTerminateSession error = %v", err)
		}
	})

	t.Run("CloseError", func(t *testing.T) {
		mockSess := &mockSession{}
		mockLife := &mockLifecycle{}
		mockSess.lifecycleFunc = func() lifecycle.Lifecycle { return mockLife }
		mockLife.closeFunc = func() error { return errors.New("close fail") }
		mockReg.getWithUserFunc = func(user string, key registry.Key) (registry.Session, error) { return mockSess, nil }

		mockStream.sendFunc = func(n *proto.Node) error {
			resp := n.GetTerminateSessionEventResponse()
			if resp.Success || resp.Message != "close fail" {
				t.Errorf("expected failure response: %v", resp)
			}
			return nil
		}
		err := c.handleTerminateSession(mockStream, evt)
		if err != nil {
			t.Errorf("handleTerminateSession error = %v", err)
		}
	})
}

func TestSubscribeAndProcess(t *testing.T) {
	mockEventSvc := &mockEventServiceClient{}
	c := &client{eventService: mockEventSvc}
	ctx := context.Background()
	backoff := time.Second

	t.Run("SubscribeError", func(t *testing.T) {
		mockEventSvc.subscribeFunc = func(ctx context.Context, opts ...grpc.CallOption) (proto.EventService_SubscribeClient, error) {
			return nil, status.Error(codes.Unauthenticated, "unauth")
		}
		err := c.subscribeAndProcess(ctx, "id", "token", &backoff)
		if !errors.Is(err, status.Error(codes.Unauthenticated, "unauth")) {
			t.Errorf("expected unauth error, got %v", err)
		}
	})

	t.Run("AuthSendError", func(t *testing.T) {
		mockStream := &mockSubscribeClient{}
		mockEventSvc.subscribeFunc = func(ctx context.Context, opts ...grpc.CallOption) (proto.EventService_SubscribeClient, error) {
			return mockStream, nil
		}
		mockStream.sendFunc = func(n *proto.Node) error {
			return status.Error(codes.Internal, "send fail")
		}
		err := c.subscribeAndProcess(ctx, "id", "token", &backoff)
		if !errors.Is(err, status.Error(codes.Internal, "send fail")) {
			t.Errorf("expected send fail, got %v", err)
		}
	})

	t.Run("StreamError", func(t *testing.T) {
		mockStream := &mockSubscribeClient{}
		mockEventSvc.subscribeFunc = func(ctx context.Context, opts ...grpc.CallOption) (proto.EventService_SubscribeClient, error) {
			return mockStream, nil
		}
		mockStream.sendFunc = func(n *proto.Node) error { return nil }
		mockStream.recvFunc = func() (*proto.Events, error) {
			return nil, status.Error(codes.Internal, "stream fail")
		}
		err := c.subscribeAndProcess(ctx, "id", "token", &backoff)
		if !errors.Is(err, status.Error(codes.Internal, "stream fail")) {
			t.Errorf("expected stream fail, got %v", err)
		}
	})
}

func TestSubscribeEvents(t *testing.T) {
	mockEventSvc := &mockEventServiceClient{}
	c := &client{eventService: mockEventSvc}

	t.Run("ReturnsOnError", func(t *testing.T) {
		mockEventSvc.subscribeFunc = func(ctx context.Context, opts ...grpc.CallOption) (proto.EventService_SubscribeClient, error) {
			return nil, errors.New("fatal error")
		}
		err := c.SubscribeEvents(context.Background(), "id", "token")
		if err == nil || err.Error() != "fatal error" {
			t.Errorf("expected fatal error, got %v", err)
		}
	})

	t.Run("RetryLoop", func(t *testing.T) {
		oldB := initialBackoff
		initialBackoff = 5 * time.Millisecond
		defer func() { initialBackoff = oldB }()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		callCount := 0
		mockEventSvc.subscribeFunc = func(ctx context.Context, opts ...grpc.CallOption) (proto.EventService_SubscribeClient, error) {
			callCount++
			return nil, status.Error(codes.Unavailable, "unavailable")
		}

		err := c.SubscribeEvents(ctx, "id", "token")
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Errorf("expected timeout/canceled error, got %v", err)
		}
		if callCount <= 1 {
			t.Errorf("expected multiple calls due to retry, got %d", callCount)
		}
	})
}

func TestCheckServerHealth(t *testing.T) {
	mockHealth := &mockHealthClient{}
	old := healthNewHealthClient
	healthNewHealthClient = func(cc grpc.ClientConnInterface) grpc_health_v1.HealthClient {
		return mockHealth
	}
	defer func() { healthNewHealthClient = old }()

	c := &client{}

	t.Run("Success", func(t *testing.T) {
		mockHealth.checkFunc = func(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
			return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
		}
		err := c.CheckServerHealth(context.Background())
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mockHealth.checkFunc = func(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
			return nil, errors.New("health fail")
		}
		err := c.CheckServerHealth(context.Background())
		if err == nil || err.Error() != "health check failed: health fail" {
			t.Errorf("expected health fail error, got %v", err)
		}
	})

	t.Run("NotServing", func(t *testing.T) {
		mockHealth.checkFunc = func(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
			return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING}, nil
		}
		err := c.CheckServerHealth(context.Background())
		if err == nil || err.Error() != "server not serving: NOT_SERVING" {
			t.Errorf("expected not serving error, got %v", err)
		}
	})
}

func TestNew_Error(t *testing.T) {
	old := grpcNewClient
	grpcNewClient = func(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
		return nil, errors.New("dial fail")
	}
	defer func() { grpcNewClient = old }()

	cli, err := New(&mockConfig{}, "localhost:1234", &mockRegistry{})
	if err == nil || err.Error() != "failed to connect to gRPC server at localhost:1234: dial fail" {
		t.Errorf("expected dial fail error, got %v", err)
	}
	if cli != nil {
		t.Errorf("expected nil client")
	}
}

func TestNew(t *testing.T) {
	mockCfg := &mockConfig{}
	mockReg := &mockRegistry{}

	cli, err := New(mockCfg, "localhost:1234", mockReg)
	if err != nil {
		t.Errorf("New() error = %v", err)
	}
	if cli == nil {
		t.Fatal("New() returned nil client")
	}
	defer cli.Close()
}

type mockConfig struct {
	config.Config
	domain string
}

func (m *mockConfig) Domain() string { return m.domain }

type mockRegistry struct {
	registry.Registry
	getFunc                   func(key registry.Key) (registry.Session, error)
	getWithUserFunc           func(user string, key registry.Key) (registry.Session, error)
	updateFunc                func(user string, oldKey, newKey registry.Key) error
	getAllSessionFromUserFunc func(user string) []registry.Session
}

func (m *mockRegistry) Get(key registry.Key) (registry.Session, error) {
	return m.getFunc(key)
}
func (m *mockRegistry) GetWithUser(user string, key registry.Key) (registry.Session, error) {
	return m.getWithUserFunc(user, key)
}
func (m *mockRegistry) Update(user string, oldKey, newKey registry.Key) error {
	return m.updateFunc(user, oldKey, newKey)
}
func (m *mockRegistry) GetAllSessionFromUser(user string) []registry.Session {
	return m.getAllSessionFromUserFunc(user)
}

type mockSession struct {
	registry.Session
	lifecycleFunc   func() lifecycle.Lifecycle
	interactionFunc func() interaction.Interaction
	detailFunc      func() *types.Detail
	slugFunc        func() slug.Slug
}

func (m *mockSession) Lifecycle() lifecycle.Lifecycle       { return m.lifecycleFunc() }
func (m *mockSession) Interaction() interaction.Interaction { return m.interactionFunc() }
func (m *mockSession) Detail() *types.Detail                { return m.detailFunc() }
func (m *mockSession) Slug() slug.Slug                      { return m.slugFunc() }

type mockInteraction struct {
	interaction.Interaction
	redrawCalled bool
}

func (m *mockInteraction) Redraw() { m.redrawCalled = true }

type mockLifecycle struct {
	lifecycle.Lifecycle
	closeFunc func() error
}

func (m *mockLifecycle) Close() error { return m.closeFunc() }

type mockEventServiceClient struct {
	proto.EventServiceClient
	subscribeFunc func(ctx context.Context, opts ...grpc.CallOption) (proto.EventService_SubscribeClient, error)
}

func (m *mockEventServiceClient) Subscribe(ctx context.Context, opts ...grpc.CallOption) (proto.EventService_SubscribeClient, error) {
	return m.subscribeFunc(ctx, opts...)
}

type mockSubscribeClient struct {
	grpc.ClientStream
	sendFunc func(*proto.Node) error
	recvFunc func() (*proto.Events, error)
}

func (m *mockSubscribeClient) Send(n *proto.Node) error     { return m.sendFunc(n) }
func (m *mockSubscribeClient) Recv() (*proto.Events, error) { return m.recvFunc() }
func (m *mockSubscribeClient) Context() context.Context     { return context.Background() }

type mockUserServiceClient struct {
	proto.UserServiceClient
	checkFunc func(ctx context.Context, in *proto.CheckRequest, opts ...grpc.CallOption) (*proto.CheckResponse, error)
}

func (m *mockUserServiceClient) Check(ctx context.Context, in *proto.CheckRequest, opts ...grpc.CallOption) (*proto.CheckResponse, error) {
	return m.checkFunc(ctx, in, opts...)
}

type mockHealthClient struct {
	grpc_health_v1.HealthClient
	checkFunc func(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error)
}

func (m *mockHealthClient) Check(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
	return m.checkFunc(ctx, in, opts...)
}

func TestProtoToTunnelType(t *testing.T) {
	c := &client{}
	tests := []struct {
		name    string
		input   proto.TunnelType
		want    types.TunnelType
		wantErr bool
	}{
		{"HTTP", proto.TunnelType_HTTP, types.TunnelTypeHTTP, false},
		{"TCP", proto.TunnelType_TCP, types.TunnelTypeTCP, false},
		{"Unknown", proto.TunnelType(999), types.TunnelTypeUNKNOWN, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.protoToTunnelType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("protoToTunnelType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("protoToTunnelType() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsConnectionError(t *testing.T) {
	c := &client{}
	tests := []struct {
		name    string
		closing bool
		err     error
		want    bool
	}{
		{"NilError", false, nil, false},
		{"Closing", true, io.EOF, false},
		{"EOF", false, io.EOF, true},
		{"Unavailable", false, status.Error(codes.Unavailable, "unavailable"), true},
		{"Canceled", false, status.Error(codes.Canceled, "canceled"), true},
		{"DeadlineExceeded", false, status.Error(codes.DeadlineExceeded, "deadline"), true},
		{"Internal", false, status.Error(codes.Internal, "internal"), false},
		{"WrappedEOF", false, errors.New("wrapped: " + io.EOF.Error()), false},
	}

	tests[7].err = fmt.Errorf("wrapped: %w", io.EOF)
	tests[7].want = true

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.closing = tt.closing
			if got := c.isConnectionError(tt.err); got != tt.want {
				t.Errorf("isConnectionError() = %v, want %v for error %v", got, tt.want, tt.err)
			}
		})
	}
}

func TestGrowBackoff(t *testing.T) {
	c := &client{}
	tests := []struct {
		name    string
		initial time.Duration
		want    time.Duration
	}{
		{"NormalGrow", time.Second, 2 * time.Second},
		{"MaxLimit", 20 * time.Second, 30 * time.Second},
		{"AlreadyAtMax", 30 * time.Second, 30 * time.Second},
		{"OverMax", 40 * time.Second, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := tt.initial
			c.growBackoff(&backoff)
			if backoff != tt.want {
				t.Errorf("growBackoff() = %v, want %v", backoff, tt.want)
			}
		})
	}
}

func TestWait(t *testing.T) {
	c := &client{}

	t.Run("ZeroDuration", func(t *testing.T) {
		err := c.wait(context.Background(), 0)
		if err != nil {
			t.Errorf("wait() zero duration error = %v", err)
		}
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := c.wait(ctx, time.Minute)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("wait() context canceled error = %v", err)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		start := time.Now()
		err := c.wait(context.Background(), 10*time.Millisecond)
		if err != nil {
			t.Errorf("wait() timeout error = %v", err)
		}
		if time.Since(start) < 10*time.Millisecond {
			t.Errorf("wait() returned too early")
		}
	})
}
