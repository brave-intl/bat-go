package nitro

import (
	"log"
	"net"
	"os"

	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/mdlayher/vsock"
)

type VsockWriter struct {
	socket net.Conn
	addr   string
}

func NewVsockWriter(addr string) *VsockWriter {
	return &VsockWriter{
		socket: nil,
		addr:   addr,
	}
}

func (w *VsockWriter) Connect() error {
	if w.socket == nil {
		s, err := Dial("tcp", w.addr)
		if err != nil {
			return err
		}
		w.socket = s
	}
	return nil
}

func (w VsockWriter) Close() error {
	if w.socket != nil {
		return w.socket.Close()
	}
	return nil
}

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

type VsockLogServer struct {
	port uint32
}

func NewVsockLogServer(port uint32) VsockLogServer {
	return VsockLogServer{port}
}

func (s VsockLogServer) Serve(l *net.Listener) error {
	if l == nil {
		l, err := vsock.Listen(s.port)
		if err != nil {
			log.Panicln(err)
		}
		defer closers.Panic(l)
	}
	log.Printf("Listening to connections on vsock port %d\n", s.port)

	for {
		conn, err := (*l).Accept()
		if err != nil {
			log.Panicln(err)
		}

		go handleLogConn(conn)
	}
}

func handleLogConn(conn net.Conn) {
	log.Println("Accepted connection.")
	defer closers.Panic(conn)
	defer log.Println("Closed connection.")

	for {
		buf := make([]byte, 1024)
		size, err := conn.Read(buf)
		if err != nil {
			return
		}
		os.Stdout.Write(buf[:size])
	}
}
