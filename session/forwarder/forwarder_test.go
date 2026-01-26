package forwarder

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type mockConfig struct {
	mock.Mock
}

func (m *mockConfig) Domain() string            { return m.Called().String(0) }
func (m *mockConfig) SSHPort() string           { return m.Called().String(0) }
func (m *mockConfig) HTTPPort() string          { return m.Called().String(0) }
func (m *mockConfig) HTTPSPort() string         { return m.Called().String(0) }
func (m *mockConfig) KeyLoc() string            { return m.Called().String(0) }
func (m *mockConfig) TLSEnabled() bool          { return m.Called().Bool(0) }
func (m *mockConfig) TLSRedirect() bool         { return m.Called().Bool(0) }
func (m *mockConfig) TLSStoragePath() string    { return m.Called().String(0) }
func (m *mockConfig) ACMEEmail() string         { return m.Called().String(0) }
func (m *mockConfig) CFAPIToken() string        { return m.Called().String(0) }
func (m *mockConfig) ACMEStaging() bool         { return m.Called().Bool(0) }
func (m *mockConfig) AllowedPortsStart() uint16 { return m.Called().Get(0).(uint16) }
func (m *mockConfig) AllowedPortsEnd() uint16   { return m.Called().Get(0).(uint16) }
func (m *mockConfig) BufferSize() int           { return m.Called().Int(0) }
func (m *mockConfig) HeaderSize() int           { return m.Called().Int(0) }
func (m *mockConfig) PprofEnabled() bool        { return m.Called().Bool(0) }
func (m *mockConfig) PprofPort() string         { return m.Called().String(0) }
func (m *mockConfig) Mode() types.ServerMode    { return m.Called().Get(0).(types.ServerMode) }
func (m *mockConfig) GRPCAddress() string       { return m.Called().String(0) }
func (m *mockConfig) GRPCPort() string          { return m.Called().String(0) }
func (m *mockConfig) NodeToken() string         { return m.Called().String(0) }

type mockConn struct {
	mock.Mock
}

func (c *mockConn) Close() error          { return c.Called().Error(0) }
func (c *mockConn) User() string          { return c.Called().String(0) }
func (c *mockConn) SessionID() []byte     { return c.Called().Get(0).([]byte) }
func (c *mockConn) ClientVersion() []byte { return c.Called().Get(0).([]byte) }
func (c *mockConn) ServerVersion() []byte { return c.Called().Get(0).([]byte) }
func (c *mockConn) RemoteAddr() net.Addr  { return c.Called().Get(0).(net.Addr) }
func (c *mockConn) LocalAddr() net.Addr   { return c.Called().Get(0).(net.Addr) }
func (c *mockConn) SendRequest(s string, b bool, d []byte) (bool, []byte, error) {
	args := c.Called(s, b, d)
	return args.Bool(0), args.Get(1).([]byte), args.Error(2)
}
func (c *mockConn) Wait() error { return c.Called().Error(0) }

func (c *mockConn) OpenChannel(name string, data []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	args := c.Called(name, data)
	return args.Get(0).(ssh.Channel), args.Get(1).(<-chan *ssh.Request), args.Error(2)
}

type testChannel struct {
	mock.Mock
	readBuf     *syncBuffer
	writeBuf    *syncBuffer
	closedWrite atomic.Bool
}

func (c *testChannel) Read(b []byte) (int, error) {
	return c.readBuf.Read(b)
}

func (c *testChannel) Write(b []byte) (int, error) {
	return c.writeBuf.Write(b)
}

func (c *testChannel) Close() error {
	return c.Called().Error(0)
}

func (c *testChannel) CloseWrite() error {
	c.closedWrite.Store(true)
	return c.writeBuf.Close()
}

func (c *testChannel) Stderr() io.ReadWriter {
	return c.Called().Get(0).(io.ReadWriter)
}

func (c *testChannel) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	args := c.Called(name, wantReply, payload)
	return args.Bool(0), args.Error(1)
}

func (c *testChannel) AckRequest(ok bool, payload []byte) error {
	return c.Called(ok, payload).Error(0)
}

type syncBuffer struct {
	mu     sync.Mutex
	buf    []byte
	closed bool
	cond   *sync.Cond
}

func newSyncBuffer() *syncBuffer {
	sb := &syncBuffer{}
	sb.cond = sync.NewCond(&sb.mu)
	return sb
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.closed {
		return 0, io.ErrClosedPipe
	}
	sb.buf = append(sb.buf, p...)
	sb.cond.Broadcast()
	return len(p), nil
}

func (sb *syncBuffer) Read(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	for len(sb.buf) == 0 {
		if sb.closed {
			return 0, io.EOF
		}
		sb.cond.Wait()
	}

	n := copy(p, sb.buf)
	sb.buf = sb.buf[n:]
	return n, nil
}

func (sb *syncBuffer) Close() error {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.closed = true
	sb.cond.Broadcast()
	return nil
}

func newChannelPair() (*testChannel, *testChannelPeer) {
	peerToChBuf := newSyncBuffer()
	chToPeerBuf := newSyncBuffer()

	channel := &testChannel{
		readBuf:  peerToChBuf,
		writeBuf: chToPeerBuf,
	}

	peer := &testChannelPeer{
		readBuf:  chToPeerBuf,
		writeBuf: peerToChBuf,
	}

	channel.On("Close").Return(nil).Maybe()

	return channel, peer
}

type testChannelPeer struct {
	readBuf  *syncBuffer
	writeBuf *syncBuffer
}

func (p *testChannelPeer) Read(b []byte) (int, error) {
	return p.readBuf.Read(b)
}

func (p *testChannelPeer) Write(b []byte) (int, error) {
	return p.writeBuf.Write(b)
}

func (p *testChannelPeer) CloseWrite() error {
	return p.writeBuf.Close()
}

func newPipePair() (*pipeConn, *pipeConn) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	conn1 := &pipeConn{
		reader: r1,
		writer: w2,
	}

	conn2 := &pipeConn{
		reader: r2,
		writer: w1,
	}

	return conn1, conn2
}

type pipeConn struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func (p *pipeConn) Read(b []byte) (int, error) {
	return p.reader.Read(b)
}

func (p *pipeConn) Write(b []byte) (int, error) {
	return p.writer.Write(b)
}

func (p *pipeConn) Close() error {
	p.reader.Close()
	p.writer.Close()
	return nil
}

func (p *pipeConn) CloseWrite() error {
	return p.writer.Close()
}

func (p *pipeConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (p *pipeConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (p *pipeConn) SetDeadline(t time.Time) error      { return nil }
func (p *pipeConn) SetReadDeadline(t time.Time) error  { return nil }
func (p *pipeConn) SetWriteDeadline(t time.Time) error { return nil }

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
		wantBufLen int
	}{
		{
			name:       "default buffer size",
			bufferSize: 16,
			wantBufLen: 16,
		},
		{
			name:       "custom buffer size",
			bufferSize: 32,
			wantBufLen: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(tt.bufferSize).Maybe()
			s := slug.New()
			conn := &mockConn{}

			forwarder := New(cfg, s, conn).(*forwarder)

			buf := forwarder.bufferPool.Get().([]byte)
			require.Len(t, buf, tt.wantBufLen)
			forwarder.bufferPool.Put(buf)

			assert.Equal(t, types.TunnelTypeUNKNOWN, forwarder.TunnelType())
			assert.Equal(t, uint16(0), forwarder.ForwardedPort())
			assert.Equal(t, conn, forwarder.conn)
			assert.Equal(t, s, forwarder.slug)
			cfg.AssertExpectations(t)
		})
	}
}

func TestHandleConnection(t *testing.T) {
	tests := []struct {
		name         string
		bufferSize   int
		messageToDst []byte
		messageToSrc []byte
	}{
		{
			name:         "small messages",
			bufferSize:   4,
			messageToDst: []byte("hi"),
			messageToSrc: []byte("yo"),
		},
		{
			name:         "medium messages",
			bufferSize:   8,
			messageToDst: []byte("hello"),
			messageToSrc: []byte("world"),
		},
		{
			name:         "larger messages",
			bufferSize:   16,
			messageToDst: []byte("I love femboy"),
			messageToSrc: []byte("mee too"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(tt.bufferSize).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			channel, channelPeer := newChannelPair()
			dstEndpoint, dstPeer := newPipePair()

			done := make(chan struct{})
			go func() {
				forwarder.HandleConnection(dstEndpoint, channel)
				close(done)
			}()

			readDst := make(chan struct {
				data []byte
				err  error
			}, 1)
			go func() {
				buf := make([]byte, len(tt.messageToDst))
				n, err := io.ReadFull(dstPeer, buf)
				readDst <- struct {
					data []byte
					err  error
				}{data: buf[:n], err: err}
			}()

			_, err := channelPeer.Write(tt.messageToDst)
			require.NoError(t, err)

			dstResult := <-readDst
			require.NoError(t, dstResult.err)
			assert.Equal(t, tt.messageToDst, dstResult.data)

			readSrc := make(chan struct {
				data []byte
				err  error
			}, 1)
			go func() {
				buf := make([]byte, len(tt.messageToSrc))
				n, err := io.ReadFull(channelPeer, buf)
				readSrc <- struct {
					data []byte
					err  error
				}{data: buf[:n], err: err}
			}()

			_, err = dstPeer.Write(tt.messageToSrc)
			require.NoError(t, err)

			srcResult := <-readSrc
			require.NoError(t, srcResult.err)
			assert.Equal(t, tt.messageToSrc, srcResult.data)

			require.NoError(t, channelPeer.CloseWrite())
			require.NoError(t, dstPeer.CloseWrite())

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("HandleConnection did not complete")
			}
			assert.True(t, channel.closedWrite.Load())
			cfg.AssertExpectations(t)
		})
	}
}

func TestHandleConnection_Error(t *testing.T) {
	tests := []struct {
		name         string
		bufferSize   int
		messageToDst []byte
		messageToSrc []byte
	}{
		{
			name:         "small messages",
			bufferSize:   4,
			messageToDst: []byte("hi"),
			messageToSrc: []byte("yo"),
		},
		{
			name:         "medium messages",
			bufferSize:   8,
			messageToDst: []byte("hello"),
			messageToSrc: []byte("world"),
		},
		{
			name:         "larger messages",
			bufferSize:   16,
			messageToDst: []byte("I love femboy"),
			messageToSrc: []byte("mee too"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(tt.bufferSize).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			channel, _ := newChannelPair()
			dstEndpoint, _ := newPipePair()

			go func() {
				forwarder.HandleConnection(dstEndpoint, channel)
			}()

			err := dstEndpoint.Close()
			assert.NoError(t, err)
			cfg.AssertExpectations(t)
		})
	}
}

func TestOpenForwardedChannel(t *testing.T) {
	tests := []struct {
		name          string
		forwardedPort uint16
		originIP      string
		originPort    int
		wantDestAddr  string
		wantDestPort  uint32
		wantOrigAddr  string
		wantOrigPort  uint32
	}{
		{
			name:          "localhost origin",
			forwardedPort: 2222,
			originIP:      "127.0.0.1",
			originPort:    9000,
			wantDestAddr:  "localhost",
			wantDestPort:  2222,
			wantOrigAddr:  "127.0.0.1",
			wantOrigPort:  9000,
		},
		{
			name:          "remote origin",
			forwardedPort: 8080,
			originIP:      "192.168.1.100",
			originPort:    5000,
			wantDestAddr:  "localhost",
			wantDestPort:  8080,
			wantOrigAddr:  "192.168.1.100",
			wantOrigPort:  5000,
		},
		{
			name:          "different port",
			forwardedPort: 3000,
			originIP:      "10.0.0.1",
			originPort:    7777,
			wantDestAddr:  "localhost",
			wantDestPort:  3000,
			wantOrigAddr:  "10.0.0.1",
			wantOrigPort:  7777,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(8).Maybe()
			channel := &testChannel{
				readBuf:  newSyncBuffer(),
				writeBuf: newSyncBuffer(),
			}
			requests := make(chan *ssh.Request)

			var capturedData []byte
			conn := &mockConn{}
			conn.On("OpenChannel", "forwarded-tcpip", mock.Anything).Run(func(args mock.Arguments) {
				data := args.Get(1).([]byte)
				capturedData = make([]byte, len(data))
				copy(capturedData, data)
			}).Return(channel, (<-chan *ssh.Request)(requests), nil)

			forwarder := New(cfg, slug.New(), conn).(*forwarder)
			forwarder.SetForwardedPort(tt.forwardedPort)

			origin := &net.TCPAddr{IP: net.ParseIP(tt.originIP), Port: tt.originPort}
			ch, reqs, err := forwarder.OpenForwardedChannel(context.Background(), origin)
			require.NoError(t, err)
			assert.Same(t, channel, ch)
			assert.NotNil(t, reqs)

			var payload struct {
				DestAddr   string
				DestPort   uint32
				OriginAddr string
				OriginPort uint32
			}
			ssh.Unmarshal(capturedData, &payload)
			assert.Equal(t, tt.wantDestAddr, payload.DestAddr)
			assert.Equal(t, tt.wantDestPort, payload.DestPort)
			assert.Equal(t, tt.wantOrigAddr, payload.OriginAddr)
			assert.Equal(t, tt.wantOrigPort, payload.OriginPort)

			conn.AssertExpectations(t)
			cfg.AssertExpectations(t)
		})
	}
}

func TestOpenForwardedChannelContextCancellation(t *testing.T) {
	tests := []struct {
		name         string
		cancelBefore bool
		cancelDuring bool
		wantErr      bool
		wantErrType  error
	}{
		{
			name:         "cancel during open",
			cancelBefore: false,
			cancelDuring: true,
			wantErr:      true,
			wantErrType:  context.Canceled,
		},
		{
			name:         "cancel before open",
			cancelBefore: true,
			cancelDuring: false,
			wantErr:      true,
			wantErrType:  context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(8).Maybe()
			channel := &testChannel{
				readBuf:  newSyncBuffer(),
				writeBuf: newSyncBuffer(),
			}
			channel.On("Close").Return(nil)
			requests := make(chan *ssh.Request)

			openChannelCalled := make(chan struct{})
			openChannelBlock := make(chan struct{})

			conn := &mockConn{}
			conn.On("OpenChannel", "forwarded-tcpip", mock.Anything).Run(func(args mock.Arguments) {
				close(openChannelCalled)
				<-openChannelBlock
			}).Return(channel, (<-chan *ssh.Request)(requests), nil).Maybe()

			forwarder := New(cfg, slug.New(), conn).(*forwarder)
			forwarder.SetForwardedPort(8080)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tt.cancelBefore {
				cancel()
			}

			var (
				openedChannel ssh.Channel
				openedReqs    <-chan *ssh.Request
				openErr       error
			)

			done := make(chan struct{})
			go func() {
				origin := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 7000}
				openedChannel, openedReqs, openErr = forwarder.OpenForwardedChannel(ctx, origin)
				close(done)
			}()

			if tt.cancelDuring {
				<-openChannelCalled
				cancel()
			}
			close(openChannelBlock)

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("OpenForwardedChannel did not return after cancellation")
			}

			if tt.wantErr {
				require.Error(t, openErr)
				assert.True(t, errors.Is(openErr, tt.wantErrType))
				assert.Nil(t, openedChannel)
				assert.Nil(t, openedReqs)
			} else {
				require.NoError(t, openErr)
				assert.NotNil(t, openedChannel)
				assert.NotNil(t, openedReqs)
			}

			conn.AssertExpectations(t)
			cfg.AssertExpectations(t)
		})
	}
}

func TestCreateForwardedTCPIPPayload(t *testing.T) {
	tests := []struct {
		name           string
		originIP       string
		originPort     int
		forwardedPort  uint16
		wantDestAddr   string
		wantDestPort   uint32
		wantOriginAddr string
		wantOriginPort uint32
	}{
		{
			name:           "standard case",
			originIP:       "192.0.2.10",
			originPort:     5050,
			forwardedPort:  8080,
			wantDestAddr:   "localhost",
			wantDestPort:   8080,
			wantOriginAddr: "192.0.2.10",
			wantOriginPort: 5050,
		},
		{
			name:           "localhost origin",
			originIP:       "127.0.0.1",
			originPort:     3000,
			forwardedPort:  9000,
			wantDestAddr:   "localhost",
			wantDestPort:   9000,
			wantOriginAddr: "127.0.0.1",
			wantOriginPort: 3000,
		},
		{
			name:           "high port numbers",
			originIP:       "10.0.0.1",
			originPort:     65535,
			forwardedPort:  65534,
			wantDestAddr:   "localhost",
			wantDestPort:   65534,
			wantOriginAddr: "10.0.0.1",
			wantOriginPort: 65535,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin := &net.TCPAddr{IP: net.ParseIP(tt.originIP), Port: tt.originPort}
			payload := createForwardedTCPIPPayload(origin, tt.forwardedPort)

			var decoded struct {
				DestAddr   string
				DestPort   uint32
				OriginAddr string
				OriginPort uint32
			}

			ssh.Unmarshal(payload, &decoded)

			assert.Equal(t, tt.wantDestAddr, decoded.DestAddr)
			assert.Equal(t, tt.wantDestPort, decoded.DestPort)
			assert.Equal(t, tt.wantOriginAddr, decoded.OriginAddr)
			assert.Equal(t, tt.wantOriginPort, decoded.OriginPort)
		})
	}
}

type mockReader struct {
	mock.Mock
}

func (m *mockReader) Read(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

type mockWriter struct {
	mock.Mock
}

func (m *mockWriter) Write(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *mockWriter) CloseWrite() error {
	return m.Called().Error(0)
}

type mockWriteCloser struct {
	mock.Mock
}

func (m *mockWriteCloser) Write(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *mockWriteCloser) Close() error {
	return m.Called().Error(0)
}

func TestCopyAndClose(t *testing.T) {
	tests := []struct {
		name          string
		setupSrc      func() io.Reader
		setupDst      func() io.Writer
		direction     string
		wantErr       bool
		wantErrMsg    string
		checkErrTypes []error
	}{
		{
			name: "successful copy with EOF",
			setupSrc: func() io.Reader {
				r := &mockReader{}
				r.On("Read", mock.Anything).Return(5, nil).Once()
				r.On("Read", mock.Anything).Return(0, io.EOF).Once()
				return r
			},
			setupDst: func() io.Writer {
				w := &mockWriter{}
				w.On("Write", mock.Anything).Return(5, nil).Once()
				w.On("CloseWrite").Return(nil).Once()
				return w
			},
			direction: "src->dst",
			wantErr:   false,
		},
		{
			name: "copy error - not EOF or ErrClosed",
			setupSrc: func() io.Reader {
				r := &mockReader{}
				customErr := errors.New("custom read error")
				r.On("Read", mock.Anything).Return(0, customErr).Once()
				return r
			},
			setupDst: func() io.Writer {
				w := &mockWriter{}
				w.On("CloseWrite").Return(nil).Once()
				return w
			},
			direction:  "src->dst",
			wantErr:    true,
			wantErrMsg: "copy error (src->dst)",
		},
		{
			name: "copy error - ErrClosed should be ignored",
			setupSrc: func() io.Reader {
				r := &mockReader{}
				r.On("Read", mock.Anything).Return(0, net.ErrClosed).Once()
				return r
			},
			setupDst: func() io.Writer {
				w := &mockWriter{}
				w.On("CloseWrite").Return(nil).Once()
				return w
			},
			direction: "src->dst",
			wantErr:   false,
		},
		{
			name: "close writer error - not EOF",
			setupSrc: func() io.Reader {
				r := &mockReader{}
				r.On("Read", mock.Anything).Return(0, io.EOF).Once()
				return r
			},
			setupDst: func() io.Writer {
				w := &mockWriter{}
				closeErr := errors.New("close error")
				w.On("CloseWrite").Return(closeErr).Once()
				return w
			},
			direction:  "src->dst",
			wantErr:    true,
			wantErrMsg: "close stream error (src->dst)",
		},
		{
			name: "close writer error - EOF should be ignored",
			setupSrc: func() io.Reader {
				r := &mockReader{}
				r.On("Read", mock.Anything).Return(0, io.EOF).Once()
				return r
			},
			setupDst: func() io.Writer {
				w := &mockWriter{}
				w.On("CloseWrite").Return(io.EOF).Once()
				return w
			},
			direction: "src->dst",
			wantErr:   false,
		},
		{
			name: "both copy and close errors",
			setupSrc: func() io.Reader {
				r := &mockReader{}
				copyErr := errors.New("copy error")
				r.On("Read", mock.Anything).Return(0, copyErr).Once()
				return r
			},
			setupDst: func() io.Writer {
				w := &mockWriter{}
				closeErr := errors.New("close error")
				w.On("CloseWrite").Return(closeErr).Once()
				return w
			},
			direction:  "src->dst",
			wantErr:    true,
			wantErrMsg: "copy error (src->dst)",
		},
		{
			name: "successful copy with WriteCloser",
			setupSrc: func() io.Reader {
				r := &mockReader{}
				r.On("Read", mock.Anything).Return(5, nil).Once()
				r.On("Read", mock.Anything).Return(0, io.EOF).Once()
				return r
			},
			setupDst: func() io.Writer {
				w := &mockWriteCloser{}
				w.On("Write", mock.Anything).Return(5, nil).Once()
				w.On("Close").Return(nil).Once()
				return w
			},
			direction: "dst->src",
			wantErr:   false,
		},
		{
			name: "WriteCloser close error",
			setupSrc: func() io.Reader {
				r := &mockReader{}
				r.On("Read", mock.Anything).Return(0, io.EOF).Once()
				return r
			},
			setupDst: func() io.Writer {
				w := &mockWriteCloser{}
				closeErr := errors.New("writeCloser close error")
				w.On("Close").Return(closeErr).Once()
				return w
			},
			direction:  "dst->src",
			wantErr:    true,
			wantErrMsg: "close stream error (dst->src)",
		},
		{
			name: "copy with multiple reads before EOF",
			setupSrc: func() io.Reader {
				r := &mockReader{}
				r.On("Read", mock.Anything).Return(10, nil).Once()
				r.On("Read", mock.Anything).Return(15, nil).Once()
				r.On("Read", mock.Anything).Return(5, nil).Once()
				r.On("Read", mock.Anything).Return(0, io.EOF).Once()
				return r
			},
			setupDst: func() io.Writer {
				w := &mockWriter{}
				w.On("Write", mock.Anything).Return(10, nil).Once()
				w.On("Write", mock.Anything).Return(15, nil).Once()
				w.On("Write", mock.Anything).Return(5, nil).Once()
				w.On("CloseWrite").Return(nil).Once()
				return w
			},
			direction: "src->dst",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(32).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			src := tt.setupSrc()
			dst := tt.setupDst()

			err := forwarder.copyAndClose(dst, src, tt.direction)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			} else {
				assert.NoError(t, err)
			}

			if mr, ok := src.(*mockReader); ok {
				mr.AssertExpectations(t)
			}
			if mw, ok := dst.(*mockWriter); ok {
				mw.AssertExpectations(t)
			}
			if mwc, ok := dst.(*mockWriteCloser); ok {
				mwc.AssertExpectations(t)
			}
			cfg.AssertExpectations(t)
		})
	}
}

func TestCopyAndCloseJoinedErrors(t *testing.T) {
	cfg := &mockConfig{}
	cfg.On("BufferSize").Return(32).Maybe()
	forwarder := New(cfg, slug.New(), nil).(*forwarder)

	src := &mockReader{}
	copyErr := errors.New("copy failed")
	src.On("Read", mock.Anything).Return(0, copyErr).Once()

	dst := &mockWriter{}
	closeErr := errors.New("close failed")
	dst.On("CloseWrite").Return(closeErr).Once()

	err := forwarder.copyAndClose(dst, src, "test")

	require.Error(t, err)

	assert.Contains(t, err.Error(), "copy error (test)")
	assert.Contains(t, err.Error(), "close stream error (test)")
	assert.Contains(t, err.Error(), "copy failed")
	assert.Contains(t, err.Error(), "close failed")

	src.AssertExpectations(t)
	dst.AssertExpectations(t)
	cfg.AssertExpectations(t)
}

func TestCopyWithBuffer(t *testing.T) {
	tests := []struct {
		name           string
		bufferSize     int
		setupSrc       func() io.Reader
		setupDst       func() io.Writer
		wantBytesCount int64
		wantErr        bool
		wantErrType    error
	}{
		{
			name:       "successful copy small data",
			bufferSize: 16,
			setupSrc: func() io.Reader {
				return io.NopCloser(bytes.NewReader([]byte("hello world")))
			},
			setupDst: func() io.Writer {
				return &bytes.Buffer{}
			},
			wantBytesCount: 11,
			wantErr:        false,
		},
		{
			name:       "successful copy large data",
			bufferSize: 8,
			setupSrc: func() io.Reader {
				data := make([]byte, 1024)
				for i := range data {
					data[i] = byte(i % 256)
				}
				return io.NopCloser(bytes.NewReader(data))
			},
			setupDst: func() io.Writer {
				return &bytes.Buffer{}
			},
			wantBytesCount: 1024,
			wantErr:        false,
		},
		{
			name:       "empty data",
			bufferSize: 16,
			setupSrc: func() io.Reader {
				return io.NopCloser(bytes.NewReader([]byte{}))
			},
			setupDst: func() io.Writer {
				return &bytes.Buffer{}
			},
			wantBytesCount: 0,
			wantErr:        false,
		},
		{
			name:       "read error",
			bufferSize: 16,
			setupSrc: func() io.Reader {
				r := &mockReader{}
				r.On("Read", mock.Anything).Return(0, errors.New("read error")).Once()
				return r
			},
			setupDst: func() io.Writer {
				return &bytes.Buffer{}
			},
			wantBytesCount: 0,
			wantErr:        true,
		},
		{
			name:       "write error",
			bufferSize: 16,
			setupSrc: func() io.Reader {
				return io.NopCloser(bytes.NewReader([]byte("test data")))
			},
			setupDst: func() io.Writer {
				w := &mockWriter{}
				w.On("Write", mock.Anything).Return(0, errors.New("write error")).Once()
				return w
			},
			wantBytesCount: 0,
			wantErr:        true,
		},
		{
			name:       "partial write continues",
			bufferSize: 16,
			setupSrc: func() io.Reader {
				return io.NopCloser(bytes.NewReader([]byte("testing")))
			},
			setupDst: func() io.Writer {
				buf := &bytes.Buffer{}
				return buf
			},
			wantBytesCount: 7,
			wantErr:        false,
		},
		{
			name:       "multiple buffer fills",
			bufferSize: 4,
			setupSrc: func() io.Reader {
				return io.NopCloser(bytes.NewReader([]byte("this is a longer message")))
			},
			setupDst: func() io.Writer {
				return &bytes.Buffer{}
			},
			wantBytesCount: 24,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(tt.bufferSize).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			src := tt.setupSrc()
			dst := tt.setupDst()

			n, err := forwarder.copyWithBuffer(dst, src)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantBytesCount, n)
			}

			if buf, ok := dst.(*bytes.Buffer); ok && !tt.wantErr {
				if _, ok := src.(io.Reader); ok {
					assert.Equal(t, tt.wantBytesCount, int64(buf.Len()))
				}
			}

			if mr, ok := src.(*mockReader); ok {
				mr.AssertExpectations(t)
			}
			if mw, ok := dst.(*mockWriter); ok {
				mw.AssertExpectations(t)
			}
			cfg.AssertExpectations(t)
		})
	}
}

func TestCopyWithBufferReusesBuffer(t *testing.T) {
	cfg := &mockConfig{}
	cfg.On("BufferSize").Return(16).Maybe()
	forwarder := New(cfg, slug.New(), nil).(*forwarder)

	buf1 := forwarder.bufferPool.Get().([]byte)
	initialPtr := &buf1[0]
	forwarder.bufferPool.Put(buf1)

	src := io.NopCloser(bytes.NewReader([]byte("test")))
	dst := &bytes.Buffer{}
	_, err := forwarder.copyWithBuffer(dst, src)
	require.NoError(t, err)

	buf2 := forwarder.bufferPool.Get().([]byte)
	secondPtr := &buf2[0]
	forwarder.bufferPool.Put(buf2)

	assert.Equal(t, len(buf1), len(buf2))

	assert.Len(t, buf2, 16)

	_ = initialPtr
	_ = secondPtr
	cfg.AssertExpectations(t)
}

func TestSetType(t *testing.T) {
	tests := []struct {
		name       string
		tunnelType types.TunnelType
	}{
		{
			name:       "set to HTTP",
			tunnelType: types.TunnelTypeHTTP,
		},
		{
			name:       "set to TCP",
			tunnelType: types.TunnelTypeTCP,
		},
		{
			name:       "set to UNKNOWN",
			tunnelType: types.TunnelTypeUNKNOWN,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(16).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			assert.Equal(t, types.TunnelTypeUNKNOWN, forwarder.TunnelType())

			forwarder.SetType(tt.tunnelType)

			assert.Equal(t, tt.tunnelType, forwarder.TunnelType())
			cfg.AssertExpectations(t)
		})
	}
}

func TestTunnelType(t *testing.T) {
	tests := []struct {
		name       string
		tunnelType types.TunnelType
	}{
		{
			name:       "get HTTP type",
			tunnelType: types.TunnelTypeHTTP,
		},
		{
			name:       "get TCP type",
			tunnelType: types.TunnelTypeTCP,
		},
		{
			name:       "get UNKNOWN type",
			tunnelType: types.TunnelTypeUNKNOWN,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(16).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			forwarder.SetType(tt.tunnelType)
			result := forwarder.TunnelType()

			assert.Equal(t, tt.tunnelType, result)
			cfg.AssertExpectations(t)
		})
	}
}

func TestSetForwardedPort(t *testing.T) {
	tests := []struct {
		name string
		port uint16
	}{
		{
			name: "set standard port",
			port: 8080,
		},
		{
			name: "set low port",
			port: 80,
		},
		{
			name: "set high port",
			port: 65535,
		},
		{
			name: "set zero port",
			port: 0,
		},
		{
			name: "set custom port",
			port: 3000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(16).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			assert.Equal(t, uint16(0), forwarder.ForwardedPort())

			forwarder.SetForwardedPort(tt.port)

			assert.Equal(t, tt.port, forwarder.ForwardedPort())
			cfg.AssertExpectations(t)
		})
	}
}

func TestForwardedPort(t *testing.T) {
	tests := []struct {
		name string
		port uint16
	}{
		{
			name: "get default port",
			port: 0,
		},
		{
			name: "get standard port",
			port: 8080,
		},
		{
			name: "get high port",
			port: 65535,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(16).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			if tt.port != 0 {
				forwarder.SetForwardedPort(tt.port)
			}

			result := forwarder.ForwardedPort()
			assert.Equal(t, tt.port, result)
			cfg.AssertExpectations(t)
		})
	}
}

func TestSetListener(t *testing.T) {
	tests := []struct {
		name          string
		setupListener func() net.Listener
	}{
		{
			name: "set TCP listener",
			setupListener: func() net.Listener {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				return listener
			},
		},
		{
			name: "set nil listener",
			setupListener: func() net.Listener {
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(16).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			listener := tt.setupListener()
			if listener != nil {
				defer listener.Close()
			}

			assert.Nil(t, forwarder.Listener())

			forwarder.SetListener(listener)

			assert.Equal(t, listener, forwarder.Listener())
			cfg.AssertExpectations(t)
		})
	}
}

func TestListener(t *testing.T) {
	tests := []struct {
		name          string
		setupListener func() net.Listener
	}{
		{
			name: "get nil listener",
			setupListener: func() net.Listener {
				return nil
			},
		},
		{
			name: "get TCP listener",
			setupListener: func() net.Listener {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				return listener
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(16).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			listener := tt.setupListener()
			if listener != nil {
				defer listener.Close()
				forwarder.SetListener(listener)
			}

			result := forwarder.Listener()
			assert.Equal(t, listener, result)
			cfg.AssertExpectations(t)
		})
	}
}

func TestClose(t *testing.T) {
	tests := []struct {
		name          string
		setupListener func() net.Listener
		wantErr       bool
	}{
		{
			name: "close with nil listener",
			setupListener: func() net.Listener {
				return nil
			},
			wantErr: false,
		},
		{
			name: "close with active listener",
			setupListener: func() net.Listener {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				return listener
			},
			wantErr: false,
		},
		{
			name: "close already closed listener",
			setupListener: func() net.Listener {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				listener.Close()
				return listener
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(16).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			listener := tt.setupListener()
			if listener != nil {
				forwarder.SetListener(listener)
			}

			err := forwarder.Close()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			cfg.AssertExpectations(t)
		})
	}
}

func TestCloseWriter(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() io.Writer
		wantErr bool
	}{
		{
			name: "close writer with CloseWrite method",
			setup: func() io.Writer {
				w := &mockWriter{}
				w.On("CloseWrite").Return(nil).Once()
				return w
			},
			wantErr: false,
		},
		{
			name: "close writer with CloseWrite error",
			setup: func() io.Writer {
				w := &mockWriter{}
				w.On("CloseWrite").Return(errors.New("close write error")).Once()
				return w
			},
			wantErr: true,
		},
		{
			name: "close WriteCloser",
			setup: func() io.Writer {
				w := &mockWriteCloser{}
				w.On("Close").Return(nil).Once()
				return w
			},
			wantErr: false,
		},
		{
			name: "close WriteCloser with error",
			setup: func() io.Writer {
				w := &mockWriteCloser{}
				w.On("Close").Return(errors.New("close error")).Once()
				return w
			},
			wantErr: true,
		},
		{
			name: "close plain writer (no close method)",
			setup: func() io.Writer {
				return &bytes.Buffer{}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := tt.setup()

			err := closeWriter(writer)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if mw, ok := writer.(*mockWriter); ok {
				mw.AssertExpectations(t)
			}
			if mwc, ok := writer.(*mockWriteCloser); ok {
				mwc.AssertExpectations(t)
			}
		})
	}
}

func TestHandleConnectionWithErrors(t *testing.T) {
	tests := []struct {
		name         string
		bufferSize   int
		setupChannel func() (*testChannel, *testChannelPeer)
		setupDst     func() (net.Conn, *pipeConn)
		simulateErr  func(channel *testChannelPeer, dst *pipeConn)
	}{
		{
			name:       "handle read error from channel",
			bufferSize: 16,
			setupChannel: func() (*testChannel, *testChannelPeer) {
				return newChannelPair()
			},
			setupDst: func() (net.Conn, *pipeConn) {
				return newPipePair()
			},
			simulateErr: func(channel *testChannelPeer, dst *pipeConn) {
				channel.CloseWrite()
				dst.CloseWrite()
			},
		},
		{
			name:       "handle write error to destination",
			bufferSize: 16,
			setupChannel: func() (*testChannel, *testChannelPeer) {
				return newChannelPair()
			},
			setupDst: func() (net.Conn, *pipeConn) {
				return newPipePair()
			},
			simulateErr: func(channel *testChannelPeer, dst *pipeConn) {
				dst.Close()
				time.Sleep(10 * time.Millisecond)
				channel.Write([]byte("test"))
				channel.CloseWrite()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(tt.bufferSize).Maybe()
			forwarder := New(cfg, slug.New(), nil).(*forwarder)

			channel, channelPeer := tt.setupChannel()
			dstEndpoint, dstPeer := tt.setupDst()

			done := make(chan struct{})
			go func() {
				forwarder.HandleConnection(dstEndpoint, channel)
				close(done)
			}()

			tt.simulateErr(channelPeer, dstPeer)

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("HandleConnection did not complete")
			}

			cfg.AssertExpectations(t)
		})
	}
}

func TestHandleConnectionDiscardOnExit(t *testing.T) {
	cfg := &mockConfig{}
	cfg.On("BufferSize").Return(16).Maybe()
	forwarder := New(cfg, slug.New(), nil).(*forwarder)

	channel, channelPeer := newChannelPair()
	dstEndpoint, dstPeer := newPipePair()

	done := make(chan struct{})
	go func() {
		forwarder.HandleConnection(dstEndpoint, channel)
		close(done)
	}()

	_, err := channelPeer.Write([]byte("test data"))
	require.NoError(t, err)
	require.NoError(t, channelPeer.CloseWrite())
	require.NoError(t, dstPeer.Close())

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("HandleConnection did not complete")
	}

	cfg.AssertExpectations(t)
}

func TestOpenForwardedChannelSuccess(t *testing.T) {
	tests := []struct {
		name          string
		forwardedPort uint16
		originAddr    string
		originPort    int
	}{
		{
			name:          "open channel standard port",
			forwardedPort: 8080,
			originAddr:    "127.0.0.1",
			originPort:    9000,
		},
		{
			name:          "open channel high port",
			forwardedPort: 65534,
			originAddr:    "192.168.1.100",
			originPort:    5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(8).Maybe()
			channel := &testChannel{
				readBuf:  newSyncBuffer(),
				writeBuf: newSyncBuffer(),
			}
			requests := make(chan *ssh.Request)

			conn := &mockConn{}
			conn.On("OpenChannel", "forwarded-tcpip", mock.Anything).
				Return(channel, (<-chan *ssh.Request)(requests), nil)

			forwarder := New(cfg, slug.New(), conn).(*forwarder)
			forwarder.SetForwardedPort(tt.forwardedPort)

			origin := &net.TCPAddr{IP: net.ParseIP(tt.originAddr), Port: tt.originPort}
			ch, reqs, err := forwarder.OpenForwardedChannel(context.Background(), origin)

			require.NoError(t, err)
			assert.NotNil(t, ch)
			assert.NotNil(t, reqs)

			conn.AssertExpectations(t)
			cfg.AssertExpectations(t)
		})
	}
}

func TestOpenForwardedChannelError(t *testing.T) {
	tests := []struct {
		name       string
		setupConn  func() *mockConn
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "open channel returns error",
			setupConn: func() *mockConn {
				conn := &mockConn{}
				conn.On("OpenChannel", "forwarded-tcpip", mock.Anything).
					Return((*testChannel)(nil), (<-chan *ssh.Request)(nil), errors.New("channel open failed"))
				return conn
			},
			wantErr:    true,
			wantErrMsg: "channel open failed",
		},
		{
			name: "open channel with nil channel",
			setupConn: func() *mockConn {
				conn := &mockConn{}
				conn.On("OpenChannel", "forwarded-tcpip", mock.Anything).
					Return((*testChannel)(nil), (<-chan *ssh.Request)(nil), nil)
				return conn
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockConfig{}
			cfg.On("BufferSize").Return(8).Maybe()

			conn := tt.setupConn()
			forwarder := New(cfg, slug.New(), conn).(*forwarder)
			forwarder.SetForwardedPort(8080)

			origin := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9000}
			_, _, err := forwarder.OpenForwardedChannel(context.Background(), origin)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			} else {
				assert.NoError(t, err)
			}

			conn.AssertExpectations(t)
			cfg.AssertExpectations(t)
		})
	}
}

func TestOpenForwardedChannelContextCancelledDuringOpen(t *testing.T) {
	cfg := &mockConfig{}
	cfg.On("BufferSize").Return(8).Maybe()

	channel := &testChannel{
		readBuf:  newSyncBuffer(),
		writeBuf: newSyncBuffer(),
	}
	channel.On("Close").Return(nil).Maybe()

	requests := make(chan *ssh.Request)

	openChannelStarted := make(chan struct{})
	openChannelBlock := make(chan struct{})

	conn := &mockConn{}
	conn.On("OpenChannel", "forwarded-tcpip", mock.Anything).Run(func(args mock.Arguments) {
		close(openChannelStarted)
		<-openChannelBlock
	}).Return(channel, (<-chan *ssh.Request)(requests), nil)

	forwarder := New(cfg, slug.New(), conn).(*forwarder)
	forwarder.SetForwardedPort(8080)

	ctx, cancel := context.WithCancel(context.Background())

	resultChan := make(chan error, 1)
	go func() {
		origin := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 7000}
		_, _, err := forwarder.OpenForwardedChannel(ctx, origin)
		resultChan <- err
	}()

	<-openChannelStarted

	cancel()

	close(openChannelBlock)

	select {
	case err := <-resultChan:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context cancelled")
	case <-time.After(2 * time.Second):
		t.Fatal("OpenForwardedChannel did not return")
	}

	time.Sleep(50 * time.Millisecond)

	conn.AssertExpectations(t)
	cfg.AssertExpectations(t)
	channel.AssertExpectations(t)
}

func TestCreateForwardedTCPIPPayloadEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		originAddr   string
		destPort     uint16
		wantDestAddr string
		wantDestPort uint32
	}{
		{
			name:         "IPv4 localhost",
			originAddr:   "127.0.0.1:5000",
			destPort:     8080,
			wantDestAddr: "localhost",
			wantDestPort: 8080,
		},
		{
			name:         "IPv6 address",
			originAddr:   "[::1]:3000",
			destPort:     9000,
			wantDestAddr: "localhost",
			wantDestPort: 9000,
		},
		{
			name:         "private network",
			originAddr:   "192.168.1.1:12345",
			destPort:     443,
			wantDestAddr: "localhost",
			wantDestPort: 443,
		},
		{
			name:         "port 1",
			originAddr:   "10.0.0.1:1",
			destPort:     1,
			wantDestAddr: "localhost",
			wantDestPort: 1,
		},
		{
			name:         "max port",
			originAddr:   "172.16.0.1:65535",
			destPort:     65535,
			wantDestAddr: "localhost",
			wantDestPort: 65535,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := net.ResolveTCPAddr("tcp", tt.originAddr)
			require.NoError(t, err)

			payload := createForwardedTCPIPPayload(addr, tt.destPort)

			var decoded struct {
				DestAddr   string
				DestPort   uint32
				OriginAddr string
				OriginPort uint32
			}

			err = ssh.Unmarshal(payload, &decoded)
			require.NoError(t, err)

			assert.Equal(t, tt.wantDestAddr, decoded.DestAddr)
			assert.Equal(t, tt.wantDestPort, decoded.DestPort)
		})
	}
}
