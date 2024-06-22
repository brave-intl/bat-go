package nitro

import (
	"context"
	"io"
	"net"
	"os"

	"github.com/brave-intl/bat-go/libs/closers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/mdlayher/vsock"
)

// VsockWriter - structure definition of a vsock writer
type VsockWriter struct {
	socket net.Conn
	addr   string
}

// NewVsockWriter - create a new vsock writer
func NewVsockWriter(addr string) io.Writer {
	if EnclaveMocking() {
		return os.Stderr
	}
	return &VsockWriter{
		socket: nil,
		addr:   addr,
	}
}

// Connect - interface implementation for connect method for VsockWriter
func (w *VsockWriter) Connect() error {
	if w.socket == nil {
		s, err := dialVsockContext(context.Background(), "tcp", w.addr)
		if err != nil {
			return err
		}
		w.socket = s
	}
	return nil
}

// Close - interface implementation of closer for VsockWriter
func (w VsockWriter) Close() error {
	if w.socket != nil {
		return w.socket.Close()
	}
	return nil
}

// Write -interface implementation of writer for VsockWriter
func (w VsockWriter) Write(p []byte) (n int, err error) {
	if w.socket == nil {
		err = w.Connect()
		if err != nil {
			return -1, err
		}
	}
	n, err = w.socket.Write(p)
	if err != nil {
		// Our socket must have disconnected, reset our connection
		w.socket = nil
		return n, err
	}
	return n, nil
}

// VsockLogServer - implementation of a log server over vsock
type VsockLogServer struct {
	baseCtx context.Context
	port    uint32
}

// NewVsockLogServer - create a new VsockLogServer
func NewVsockLogServer(ctx context.Context, port uint32) VsockLogServer {
	return VsockLogServer{
		baseCtx: ctx,
		port:    port,
	}
}

// Serve - interface implementation for Serve for VsockLogServer
func (s VsockLogServer) Serve(l net.Listener) error {
	logger := logging.Logger(s.baseCtx, "nitro.Serve")
	if l == nil {
		var err error
		l, err = vsock.Listen(s.port, &vsock.Config{})
		if err != nil {
			return err
		}
		defer closers.Panic(s.baseCtx, l)
	}
	logger.Info().Uint32("port", s.port).Msg("Listening to connections on vsock port")

	for {
		conn, err := l.Accept()
		if err != nil {
			logger.Error().Err(err).Msg("accept failed")
		}

		go handleLogConn(s.baseCtx, conn)
	}
}

func handleLogConn(ctx context.Context, conn net.Conn) {
	logger := logging.Logger(ctx, "nitro.handleLogConn")

	logger.Debug().Msg("Accepted connection.")
	defer closers.Panic(ctx, conn)
	defer logger.Debug().Msg("Closed connection.")

	for {
		buf := make([]byte, 1024)
		size, err := conn.Read(buf)
		if err != nil {
			return
		}
		if _, err := os.Stdout.Write(buf[:size]); err != nil {
			logger.Error().Err(err).Msg("failed to write")
		}
	}
}
