package web

import (
	"context"
	"fmt"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ProcessSource interface {
	ListProcesses(ctx context.Context) ([]*pb.Process, error)
}

type GRPCProcessSource struct {
	Port    int
	Timeout time.Duration
}

func NewGRPCProcessSource(port int) GRPCProcessSource {
	if port == 0 {
		port = DefaultDaemonPort
	}
	return GRPCProcessSource{
		Port:    port,
		Timeout: 2 * time.Second,
	}
}

func (source GRPCProcessSource) ListProcesses(ctx context.Context) ([]*pb.Process, error) {
	timeout := source.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := grpc.NewClient(
		fmt.Sprintf("127.0.0.1:%d", source.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	manager := pb.NewProcessManagerClient(conn)
	resp, err := manager.ListProcess(ctx, &pb.ListProcessRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetProcesses(), nil
}
