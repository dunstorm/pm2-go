package server

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type Handler struct {
	logger         *zerolog.Logger
	databaseById   map[int32]*pb.Process
	databaseByName map[string]*pb.Process
	mu             sync.Mutex

	processes        map[int32]*os.Process
	metricsUpdatedAt map[int32]time.Time
	nextId           int32

	pb.UnimplementedProcessManagerServer
}

func New(port int) {
	s, lis, err := NewServer(port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func NewServer(port int) (*grpc.Server, net.Listener, error) {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	lis, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, nil, err
	}
	s := grpc.NewServer()
	handler := &Handler{
		logger:           &logger,
		databaseById:     make(map[int32]*pb.Process, 0),
		databaseByName:   make(map[string]*pb.Process, 0),
		processes:        make(map[int32]*os.Process, 0),
		metricsUpdatedAt: make(map[int32]time.Time),
	}
	pb.RegisterProcessManagerServer(s, handler)

	handler.restoreState()
	startScheduler(handler)

	handler.logger.Info().Msgf("Serving GRPC server at %s", lis.Addr())

	return s, lis, nil
}
