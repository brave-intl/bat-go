package nitro

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestServe(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:1234")
	if err != nil {
		t.Error("Unexpected error listening")
	}
	s := NewVsockLogServer(context.Background(), 1234)
	go func() {
		if err := s.Serve(l); err != nil {
			t.Error("failed to serve log server")
		}
	}()

	log := zerolog.New(NewVsockWriter("127.0.0.1:1234"))
	log.Info().Msg("hello world")
	time.Sleep(1000 * time.Millisecond)

	log = zerolog.New(zerolog.ConsoleWriter{Out: NewVsockWriter("127.0.0.1:1234")})
	log.Info().Msg("hello world")
	time.Sleep(1000 * time.Millisecond)
}
