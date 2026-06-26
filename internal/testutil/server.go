package testutil

import (
	"net"
	"testing"
	"time"

	"github.com/dunstorm/pm2-go/internal/grpc/server"
	"github.com/dunstorm/pm2-go/internal/utils"
)

func StartGRPCServer(t testing.TB) int {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	grpcServer, lis, err := server.NewServer(0)
	if err != nil {
		t.Fatalf("start grpc test server: %v", err)
	}

	port := lis.Addr().(*net.TCPAddr).Port
	go func() {
		_ = grpcServer.Serve(lis)
	}()

	for i := 0; i < 100; i++ {
		if utils.IsPortOpen(port) {
			t.Cleanup(grpcServer.Stop)
			return port
		}
		time.Sleep(10 * time.Millisecond)
	}

	grpcServer.Stop()
	t.Fatalf("grpc test server did not open port %d", port)
	return 0
}
