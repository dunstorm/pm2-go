package server

import (
	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/protobuf/proto"
)

func cloneProcess(process *pb.Process) *pb.Process {
	if process == nil {
		return nil
	}
	return proto.Clone(process).(*pb.Process)
}
