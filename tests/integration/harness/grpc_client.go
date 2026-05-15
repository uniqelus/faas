package harness

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func DialGRPC(ctx context.Context, target string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("init grpc client %s: %w", target, err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return conn, nil
		}
		if !conn.WaitForStateChange(connectCtx, state) {
			_ = conn.Close()
			return nil, fmt.Errorf("connect grpc %s: %w", target, connectCtx.Err())
		}
	}
}
