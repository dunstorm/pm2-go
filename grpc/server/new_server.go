package server

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type Handler struct {
	logger         *zerolog.Logger
	databaseById   map[int32]*pb.Process
	databaseByName map[string]*pb.Process
	mu             sync.Mutex

	processes map[int32]*os.Process
	nextId    int32

	pb.UnimplementedProcessManagerServer
}

func New(port int) {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	lis, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	handler := &Handler{
		logger:         &logger,
		databaseById:   make(map[int32]*pb.Process, 0),
		databaseByName: make(map[string]*pb.Process, 0),
		processes:      make(map[int32]*os.Process, 0),
	}
	pb.RegisterProcessManagerServer(s, handler)

	startScheduler(handler)

	handler.logger.Info().Msgf("Serving GRPC server at %s", lis.Addr())

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
