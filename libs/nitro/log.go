package nitro

import (
	"context"
	"log"
	"net"
	"os"

	"github.com/brave-intl/bat-go/libs/closers"
	"github.com/mdlayher/vsock"
)

// VsockWriter - structure definition of a vsock writer
type VsockWriter struct {
	socket net.Conn
	addr   string
}

// NewVsockWriter - create a new vsock writer
func NewVsockWriter(addr string) *VsockWriter {
	return &VsockWriter{
		socket: nil,
		addr:   addr,
	}
}

// Connect - interface implementation for connect method for VsockWriter
func (w *VsockWriter) Connect() error {
	if w.socket == nil {
		s, err := DialContext(context.Background(), "tcp", w.addr)
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
	if l == nil {
		var err error
		l, err = vsock.Listen(s.port)
		if err != nil {
			log.Panicln(err)
		}
		defer closers.Panic(s.baseCtx, l)
	}
	log.Printf("Listening to connections on vsock port %d\n", s.port)

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Panicln(err)
		}

		go handleLogConn(s.baseCtx, conn)
	}
}

func handleLogConn(ctx context.Context, conn net.Conn) {
	log.Println("Accepted connection.")
	defer closers.Panic(ctx, conn)
	defer log.Println("Closed connection.")

	for {
		buf := make([]byte, 1024)
		size, err := conn.Read(buf)
		if err != nil {
			return
		}
		if _, err := os.Stdout.Write(buf[:size]); err != nil {
			log.Printf("failed to write: %s", err.Error())
		}
	}
}
