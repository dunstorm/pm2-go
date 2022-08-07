package client

import (
	"context"
	"fmt"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"github.com/dunstorm/pm2-go/utils"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	port int

	logger *zerolog.Logger
}

func New(port int) (*Client, error) {

	logger := utils.NewLogger()
	client := &Client{
		port:   port,
		logger: logger,
	}

	return client, nil
}

func (c *Client) Dial() (*grpc.ClientConn, *pb.ProcessManagerClient) {
	// Set up a connection to the server.
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", c.port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		c.logger.Fatal().Msgf("did not connect: %v", err)
	}
	client := pb.NewProcessManagerClient(conn)
	return conn, &client
}

// create process
func (c *Client) AddProcess(request *pb.AddProcessRequest) int32 {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, manager := c.Dial()
	defer conn.Close()
	r, err := (*manager).AddProcess(ctx, request)
	if err != nil {
		c.logger.Fatal().Msgf("%s", err.Error())
	}
	return r.GetId()
}

// find process
func (c *Client) FindProcess(name string) *pb.Process {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, manager := c.Dial()
	defer conn.Close()
	r, err := (*manager).FindProcess(ctx, &pb.FindProcessRequest{Name: name})
	if err != nil {
		return nil
	}
	return r
}

// stop process
func (c *Client) StopProcess(index int32) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, manager := c.Dial()
	defer conn.Close()
	r, err := (*manager).StopProcess(ctx, &pb.StopProcessRequest{Id: index})
	if err != nil {
		c.logger.Fatal().Msgf("%s", err.Error())
	}
	return r.GetSuccess()
}

// list processes
func (c *Client) ListProcess() []*pb.Process {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, manager := c.Dial()
	defer conn.Close()
	r, err := (*manager).ListProcess(ctx, &pb.ListProcessRequest{})
	if err != nil {
		c.logger.Fatal().Msgf("%s", err.Error())
	}
	return r.GetProcesses()
}

// update process
func (c *Client) StartProcess(request *pb.StartProcessRequest) *pb.Process {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, manager := c.Dial()
	defer conn.Close()
	r, err := (*manager).StartProcess(ctx, request)
	if err != nil {
		c.logger.Fatal().Msgf("%s", err.Error())
	}
	return r
}

// delete process
func (c *Client) DeleteProcess(id int32) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, manager := c.Dial()
	defer conn.Close()
	r, err := (*manager).DeleteProcess(ctx, &pb.DeleteProcessRequest{Id: id})
	if err != nil {
		c.logger.Fatal().Msgf("%s", err.Error())
	}
	return r.GetSuccess()
}
