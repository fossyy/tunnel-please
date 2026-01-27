package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"tunnel_pls/internal/port"
	"tunnel_pls/internal/registry"
	"tunnel_pls/session/forwarder"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	proto "git.fossy.my.id/bagas/tunnel-please-grpc/gen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/ssh"
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
			mockUserSvc.On("Check", mock.Anything, &proto.CheckRequest{AuthToken: tt.token}, mock.Anything).Return(tt.mockResp, tt.mockErr).Once()

			auth, user, err := c.AuthorizeConn(context.Background(), tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("AuthorizeConn() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.wantAuth, auth)
			assert.Equal(t, tt.wantUser, user)
			mockUserSvc.AssertExpectations(t)
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
	c := &client{}

	t.Run("UnknownEventType", func(t *testing.T) {
		mockStream := &mockSubscribeClient{}
		mockStream.On("Recv").Return(&proto.Events{Type: proto.EventType(999)}, nil).Once()
		mockStream.On("Recv").Return(nil, io.EOF).Once()

		err := c.processEventStream(mockStream)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("DispatchSuccess", func(t *testing.T) {
		events := []proto.EventType{
			proto.EventType_SLUG_CHANGE,
			proto.EventType_GET_SESSIONS,
			proto.EventType_TERMINATE_SESSION,
		}

		for _, et := range events {
			t.Run(et.String(), func(t *testing.T) {
				mockStream := &mockSubscribeClient{}
				payload := &proto.Events{Type: et}
				switch et {
				case proto.EventType_SLUG_CHANGE:
					payload.Payload = &proto.Events_SlugEvent{SlugEvent: &proto.SlugChangeEvent{}}
				case proto.EventType_GET_SESSIONS:
					payload.Payload = &proto.Events_GetSessionsEvent{GetSessionsEvent: &proto.GetSessionsEvent{}}
				case proto.EventType_TERMINATE_SESSION:
					payload.Payload = &proto.Events_TerminateSessionEvent{TerminateSessionEvent: &proto.TerminateSessionEvent{}}
				}

				mockStream.On("Recv").Return(payload, nil).Once()
				mockStream.On("Recv").Return(nil, io.EOF).Once()

				mockReg := &mockRegistry{}
				c.sessionRegistry = mockReg
				mCfg := &MockConfig{}
				c.config = mCfg
				mCfg.On("Domain").Return("test.com").Maybe()

				switch et {
				case proto.EventType_SLUG_CHANGE:
					mockReg.On("Get", mock.Anything).Return(nil, errors.New("fail")).Once()
				case proto.EventType_GET_SESSIONS:
					mockReg.On("GetAllSessionFromUser", mock.Anything).Return(nil).Once()
				case proto.EventType_TERMINATE_SESSION:
					mockReg.On("GetWithUser", mock.Anything, mock.Anything).Return(nil, errors.New("fail")).Once()
				}
				mockStream.On("Send", mock.Anything).Return(nil).Once()

				err := c.processEventStream(mockStream)
				assert.ErrorIs(t, err, io.EOF)
			})
		}
	})

	t.Run("HandlerError", func(t *testing.T) {
		mockStream := &mockSubscribeClient{}
		mockStream.On("Recv").Return(&proto.Events{
			Type:    proto.EventType_SLUG_CHANGE,
			Payload: &proto.Events_SlugEvent{SlugEvent: &proto.SlugChangeEvent{}},
		}, nil).Once()

		mockReg := &mockRegistry{}
		mockReg.On("Get", mock.Anything).Return(nil, errors.New("fail")).Once()
		c.sessionRegistry = mockReg

		expectedErr := status.Error(codes.Unavailable, "send fail")
		mockStream.On("Send", mock.Anything).Return(expectedErr).Once()

		err := c.processEventStream(mockStream)
		assert.Equal(t, expectedErr, err)
	})
}

func TestSendNode(t *testing.T) {
	c := &client{}

	t.Run("Success", func(t *testing.T) {
		mockStream := &mockSubscribeClient{}
		mockStream.On("Send", mock.Anything).Return(nil).Once()
		err := c.sendNode(mockStream, &proto.Node{}, "context")
		assert.NoError(t, err)
		mockStream.AssertExpectations(t)
	})

	t.Run("ConnectionError", func(t *testing.T) {
		mockStream := &mockSubscribeClient{}
		expectedErr := status.Error(codes.Unavailable, "fail")
		mockStream.On("Send", mock.Anything).Return(expectedErr).Once()
		err := c.sendNode(mockStream, &proto.Node{}, "context")
		assert.ErrorIs(t, err, expectedErr)
		mockStream.AssertExpectations(t)
	})

	t.Run("OtherError", func(t *testing.T) {
		mockStream := &mockSubscribeClient{}
		mockStream.On("Send", mock.Anything).Return(status.Error(codes.Internal, "fail")).Once()
		err := c.sendNode(mockStream, &proto.Node{}, "context")
		assert.NoError(t, err)
		mockStream.AssertExpectations(t)
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
		mockSess.On("Interaction").Return(mockInter).Once()
		mockInter.On("Redraw").Return().Once()

		mockReg.On("Get", types.SessionKey{Id: "old-slug", Type: types.TunnelTypeHTTP}).Return(mockSess, nil).Once()
		mockReg.On("Update", "mas-fuad", types.SessionKey{Id: "old-slug", Type: types.TunnelTypeHTTP}, types.SessionKey{Id: "new-slug", Type: types.TunnelTypeHTTP}).Return(nil).Once()

		mockStream.On("Send", mock.MatchedBy(func(n *proto.Node) bool {
			return n.Type == proto.EventType_SLUG_CHANGE_RESPONSE && n.GetSlugEventResponse().Success
		})).Return(nil).Once()

		err := c.handleSlugChange(mockStream, evt)
		assert.NoError(t, err)
		mockReg.AssertExpectations(t)
		mockStream.AssertExpectations(t)
		mockInter.AssertExpectations(t)
	})

	t.Run("SessionNotFound", func(t *testing.T) {
		mockReg.On("Get", mock.Anything).Return(nil, errors.New("not found")).Once()
		mockStream.On("Send", mock.MatchedBy(func(n *proto.Node) bool {
			return !n.GetSlugEventResponse().Success && n.GetSlugEventResponse().Message == "not found"
		})).Return(nil).Once()

		err := c.handleSlugChange(mockStream, evt)
		assert.NoError(t, err)
		mockReg.AssertExpectations(t)
		mockStream.AssertExpectations(t)
	})

	t.Run("UpdateError", func(t *testing.T) {
		mockSess := &mockSession{}
		mockReg.On("Get", mock.Anything).Return(mockSess, nil).Once()
		mockReg.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(errors.New("update fail")).Once()
		mockStream.On("Send", mock.MatchedBy(func(n *proto.Node) bool {
			return !n.GetSlugEventResponse().Success && n.GetSlugEventResponse().Message == "update fail"
		})).Return(nil).Once()

		err := c.handleSlugChange(mockStream, evt)
		assert.NoError(t, err)
		mockReg.AssertExpectations(t)
		mockStream.AssertExpectations(t)
	})
}

func TestHandleGetSessions(t *testing.T) {
	mockReg := &mockRegistry{}
	mockStream := &mockSubscribeClient{}
	mockCfg := &MockConfig{}
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
		mockSess.On("Detail").Return(&types.Detail{
			ForwardingType: "http",
			Slug:           "myslug",
			UserID:         "mas-fuad",
			Active:         true,
			StartedAt:      now,
		}).Once()

		mockReg.On("GetAllSessionFromUser", "mas-fuad").Return([]registry.Session{mockSess}).Once()
		mockCfg.On("Domain").Return("test.com").Once()

		mockStream.On("Send", mock.MatchedBy(func(n *proto.Node) bool {
			if n.Type != proto.EventType_GET_SESSIONS {
				return false
			}
			details := n.GetGetSessionsEvent().Details
			return len(details) == 1 && details[0].Slug == "myslug"
		})).Return(nil).Once()

		err := c.handleGetSessions(mockStream, evt)
		assert.NoError(t, err)
		mockReg.AssertExpectations(t)
		mockStream.AssertExpectations(t)
		mockCfg.AssertExpectations(t)
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
		mockSess.On("Lifecycle").Return(mockLife).Once()
		mockLife.On("Close").Return(nil).Once()

		mockReg.On("GetWithUser", "mas-fuad", types.SessionKey{Id: "myslug", Type: types.TunnelTypeHTTP}).Return(mockSess, nil).Once()

		mockStream.On("Send", mock.MatchedBy(func(n *proto.Node) bool {
			return n.GetTerminateSessionEventResponse().Success
		})).Return(nil).Once()

		err := c.handleTerminateSession(mockStream, evt)
		assert.NoError(t, err)
		mockReg.AssertExpectations(t)
		mockStream.AssertExpectations(t)
		mockLife.AssertExpectations(t)
	})

	t.Run("TunnelTypeUnknown", func(t *testing.T) {
		badEvt := &proto.Events{
			Payload: &proto.Events_TerminateSessionEvent{
				TerminateSessionEvent: &proto.TerminateSessionEvent{
					TunnelType: proto.TunnelType(999),
				},
			},
		}
		mockStream.On("Send", mock.MatchedBy(func(n *proto.Node) bool {
			resp := n.GetTerminateSessionEventResponse()
			return !resp.Success && resp.Message != ""
		})).Return(nil).Once()

		err := c.handleTerminateSession(mockStream, badEvt)
		assert.NoError(t, err)
		mockStream.AssertExpectations(t)
	})

	t.Run("SessionNotFound", func(t *testing.T) {
		mockReg.On("GetWithUser", mock.Anything, mock.Anything).Return(nil, errors.New("not found")).Once()
		mockStream.On("Send", mock.MatchedBy(func(n *proto.Node) bool {
			resp := n.GetTerminateSessionEventResponse()
			return !resp.Success && resp.Message == "not found"
		})).Return(nil).Once()

		err := c.handleTerminateSession(mockStream, evt)
		assert.NoError(t, err)
		mockReg.AssertExpectations(t)
		mockStream.AssertExpectations(t)
	})

	t.Run("CloseError", func(t *testing.T) {
		mockSess := &mockSession{}
		mockLife := &mockLifecycle{}
		mockSess.On("Lifecycle").Return(mockLife).Once()
		mockLife.On("Close").Return(errors.New("close fail")).Once()
		mockReg.On("GetWithUser", mock.Anything, mock.Anything).Return(mockSess, nil).Once()

		mockStream.On("Send", mock.MatchedBy(func(n *proto.Node) bool {
			resp := n.GetTerminateSessionEventResponse()
			return !resp.Success && resp.Message == "close fail"
		})).Return(nil).Once()

		err := c.handleTerminateSession(mockStream, evt)
		assert.NoError(t, err)
		mockReg.AssertExpectations(t)
		mockStream.AssertExpectations(t)
		mockLife.AssertExpectations(t)
	})
}

func TestSubscribeAndProcess(t *testing.T) {
	mockEventSvc := &mockEventServiceClient{}
	c := &client{eventService: mockEventSvc}
	ctx := context.Background()
	backoff := time.Second

	t.Run("SubscribeError", func(t *testing.T) {
		expectedErr := status.Error(codes.Unauthenticated, "unauth")
		mockEventSvc.On("Subscribe", mock.Anything, mock.Anything).Return(nil, expectedErr).Once()
		err := c.subscribeAndProcess(ctx, "id", "token", &backoff)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("AuthSendError", func(t *testing.T) {
		mockStream := &mockSubscribeClient{}
		mockEventSvc.On("Subscribe", mock.Anything, mock.Anything).Return(mockStream, nil).Once()
		expectedErr := status.Error(codes.Internal, "send fail")
		mockStream.On("Send", mock.Anything).Return(expectedErr).Once()
		err := c.subscribeAndProcess(ctx, "id", "token", &backoff)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("StreamError", func(t *testing.T) {
		mockStream := &mockSubscribeClient{}
		mockEventSvc.On("Subscribe", mock.Anything, mock.Anything).Return(mockStream, nil).Once()
		mockStream.On("Send", mock.Anything).Return(nil).Once()
		expectedErr := status.Error(codes.Internal, "stream fail")
		mockStream.On("Recv").Return(nil, expectedErr).Once()
		err := c.subscribeAndProcess(ctx, "id", "token", &backoff)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestSubscribeEvents(t *testing.T) {
	mockEventSvc := &mockEventServiceClient{}
	c := &client{eventService: mockEventSvc}

	t.Run("ReturnsOnError", func(t *testing.T) {
		expectedErr := errors.New("fatal error")
		mockEventSvc.On("Subscribe", mock.Anything, mock.Anything).Return(nil, expectedErr).Once()
		err := c.SubscribeEvents(context.Background(), "id", "token")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("RetryLoop", func(t *testing.T) {
		oldB := initialBackoff
		initialBackoff = 5 * time.Millisecond
		defer func() { initialBackoff = oldB }()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		mockEventSvc.On("Subscribe", mock.Anything, mock.Anything).Return(nil, status.Error(codes.Unavailable, "unavailable"))

		err := c.SubscribeEvents(ctx, "id", "token")
		assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
		mockEventSvc.AssertExpectations(t)
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
		mockHealth.On("Check", mock.Anything, mock.Anything, mock.Anything).Return(&grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil).Once()
		err := c.CheckServerHealth(context.Background())
		assert.NoError(t, err)
		mockHealth.AssertExpectations(t)
	})

	t.Run("Error", func(t *testing.T) {
		mockHealth.On("Check", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("health fail")).Once()
		err := c.CheckServerHealth(context.Background())
		assert.ErrorContains(t, err, "health check failed: health fail")
		mockHealth.AssertExpectations(t)
	})

	t.Run("NotServing", func(t *testing.T) {
		mockHealth.On("Check", mock.Anything, mock.Anything, mock.Anything).Return(&grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING}, nil).Once()
		err := c.CheckServerHealth(context.Background())
		assert.ErrorContains(t, err, "server not serving: NOT_SERVING")
		mockHealth.AssertExpectations(t)
	})
}

func TestNew_Error(t *testing.T) {
	old := grpcNewClient
	grpcNewClient = func(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
		return nil, errors.New("dial fail")
	}
	defer func() { grpcNewClient = old }()
	mockConfig := &MockConfig{}

	mockConfig.On("GRPCAddress").Return("localhost")
	mockConfig.On("GRPCPort").Return("1234")
	cli, err := New(mockConfig, &mockRegistry{})
	if err == nil || err.Error() != "failed to connect to gRPC server at localhost:1234: dial fail" {
		t.Errorf("expected dial fail error, got %v", err)
	}
	if cli != nil {
		t.Errorf("expected nil client")
	}
}

func TestNew(t *testing.T) {
	mockConfig := &MockConfig{}
	mockReg := &mockRegistry{}
	mockConfig.On("GRPCAddress").Return("localhost")
	mockConfig.On("GRPCPort").Return("1234")
	cli, err := New(mockConfig, mockReg)
	if err != nil {
		t.Errorf("New() error = %v", err)
	}
	if cli == nil {
		t.Fatal("New() returned nil client")
	}
	defer func(cli Client) {
		_ = cli.Close()
	}(cli)
}

type MockConfig struct {
	mock.Mock
}

func (m *MockConfig) Domain() string            { return m.Called().String(0) }
func (m *MockConfig) SSHPort() string           { return m.Called().String(0) }
func (m *MockConfig) HTTPPort() string          { return m.Called().String(0) }
func (m *MockConfig) HTTPSPort() string         { return m.Called().String(0) }
func (m *MockConfig) TLSEnabled() bool          { return m.Called().Bool(0) }
func (m *MockConfig) TLSRedirect() bool         { return m.Called().Bool(0) }
func (m *MockConfig) TLSStoragePath() string    { return m.Called().String(0) }
func (m *MockConfig) ACMEEmail() string         { return m.Called().String(0) }
func (m *MockConfig) CFAPIToken() string        { return m.Called().String(0) }
func (m *MockConfig) ACMEStaging() bool         { return m.Called().Bool(0) }
func (m *MockConfig) AllowedPortsStart() uint16 { return uint16(m.Called().Int(0)) }
func (m *MockConfig) AllowedPortsEnd() uint16   { return uint16(m.Called().Int(0)) }
func (m *MockConfig) BufferSize() int           { return m.Called().Int(0) }
func (m *MockConfig) HeaderSize() int           { return m.Called().Int(0) }
func (m *MockConfig) PprofEnabled() bool        { return m.Called().Bool(0) }
func (m *MockConfig) PprofPort() string         { return m.Called().String(0) }
func (m *MockConfig) Mode() types.ServerMode    { return m.Called().Get(0).(types.ServerMode) }
func (m *MockConfig) GRPCAddress() string       { return m.Called().String(0) }
func (m *MockConfig) GRPCPort() string          { return m.Called().String(0) }
func (m *MockConfig) NodeToken() string         { return m.Called().String(0) }
func (m *MockConfig) KeyLoc() string            { return m.Called().String(0) }

type mockRegistry struct {
	mock.Mock
}

func (m *mockRegistry) Get(key registry.Key) (registry.Session, error) {
	args := m.Called(key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(registry.Session), args.Error(1)
}
func (m *mockRegistry) GetWithUser(user string, key registry.Key) (registry.Session, error) {
	args := m.Called(user, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(registry.Session), args.Error(1)
}
func (m *mockRegistry) Update(user string, oldKey, newKey registry.Key) error {
	return m.Called(user, oldKey, newKey).Error(0)
}
func (m *mockRegistry) GetAllSessionFromUser(user string) []registry.Session {
	args := m.Called(user)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]registry.Session)
}
func (m *mockRegistry) Register(key registry.Key, session registry.Session) bool {
	return m.Called(key, session).Bool(0)
}
func (m *mockRegistry) Remove(key registry.Key) {
	m.Called(key)
}

type mockSession struct {
	mock.Mock
}

func (m *mockSession) Lifecycle() lifecycle.Lifecycle {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(lifecycle.Lifecycle)
}
func (m *mockSession) Interaction() interaction.Interaction {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(interaction.Interaction)
}
func (m *mockSession) Detail() *types.Detail {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*types.Detail)
}
func (m *mockSession) Slug() slug.Slug {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(slug.Slug)
}
func (m *mockSession) Forwarder() forwarder.Forwarder {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(forwarder.Forwarder)
}

type mockInteraction struct {
	mock.Mock
}

func (m *mockInteraction) Start()                             { m.Called() }
func (m *mockInteraction) Stop()                              { m.Called() }
func (m *mockInteraction) Redraw()                            { m.Called() }
func (m *mockInteraction) SetWH(w, h int)                     { m.Called(w, h) }
func (m *mockInteraction) SetChannel(channel ssh.Channel)     { m.Called(channel) }
func (m *mockInteraction) SetMode(mode types.InteractiveMode) { m.Called(mode) }
func (m *mockInteraction) Mode() types.InteractiveMode {
	return m.Called().Get(0).(types.InteractiveMode)
}
func (m *mockInteraction) Send(message string) error { return m.Called(message).Error(0) }

type mockLifecycle struct {
	mock.Mock
}

func (m *mockLifecycle) Close() error { return m.Called().Error(0) }
func (m *mockLifecycle) Channel() ssh.Channel {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(ssh.Channel)
}
func (m *mockLifecycle) Connection() ssh.Conn {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(ssh.Conn)
}
func (m *mockLifecycle) User() string                         { return m.Called().String(0) }
func (m *mockLifecycle) SetChannel(channel ssh.Channel)       { m.Called(channel) }
func (m *mockLifecycle) SetStatus(status types.SessionStatus) { m.Called(status) }
func (m *mockLifecycle) IsActive() bool                       { return m.Called().Bool(0) }
func (m *mockLifecycle) StartedAt() time.Time                 { return m.Called().Get(0).(time.Time) }
func (m *mockLifecycle) PortRegistry() port.Port {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(port.Port)
}

type mockEventServiceClient struct {
	mock.Mock
}

func (m *mockEventServiceClient) Subscribe(ctx context.Context, opts ...grpc.CallOption) (proto.EventService_SubscribeClient, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(proto.EventService_SubscribeClient), args.Error(1)
}

type mockSubscribeClient struct {
	mock.Mock
	grpc.ClientStream
}

func (m *mockSubscribeClient) Send(n *proto.Node) error { return m.Called(n).Error(0) }
func (m *mockSubscribeClient) Recv() (*proto.Events, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*proto.Events), args.Error(1)
}
func (m *mockSubscribeClient) Context() context.Context { return m.Called().Get(0).(context.Context) }

type mockUserServiceClient struct {
	mock.Mock
}

func (m *mockUserServiceClient) Check(ctx context.Context, in *proto.CheckRequest, opts ...grpc.CallOption) (*proto.CheckResponse, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*proto.CheckResponse), args.Error(1)
}

type mockHealthClient struct {
	mock.Mock
}

func (m *mockHealthClient) Check(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthCheckResponse, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*grpc_health_v1.HealthCheckResponse), args.Error(1)
}

func (m *mockHealthClient) Watch(ctx context.Context, in *grpc_health_v1.HealthCheckRequest, opts ...grpc.CallOption) (grpc_health_v1.Health_WatchClient, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(grpc_health_v1.Health_WatchClient), args.Error(1)
}

func (m *mockHealthClient) List(ctx context.Context, in *grpc_health_v1.HealthListRequest, opts ...grpc.CallOption) (*grpc_health_v1.HealthListResponse, error) {
	args := m.Called(ctx, in, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*grpc_health_v1.HealthListResponse), args.Error(1)
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
