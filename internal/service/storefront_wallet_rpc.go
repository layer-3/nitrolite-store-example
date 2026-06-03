package service

import (
	"context"
	"fmt"

	"github.com/layer-3/nitrolite/pkg/rpc"
)

func createAppSessionRPC(ctx context.Context, wsURL string, req rpc.AppSessionsV1CreateAppSessionRequest) (*rpc.AppSessionsV1CreateAppSessionResponse, error) {
	var out rpc.AppSessionsV1CreateAppSessionResponse
	err := withRPCClient(ctx, wsURL, func(client *rpc.Client) error {
		resp, err := client.AppSessionsV1CreateAppSession(ctx, req)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func submitAppStateRPC(ctx context.Context, wsURL string, req rpc.AppSessionsV1SubmitAppStateRequest) error {
	return withRPCClient(ctx, wsURL, func(client *rpc.Client) error {
		_, err := client.AppSessionsV1SubmitAppState(ctx, req)
		return err
	})
}

func withRPCClient(ctx context.Context, wsURL string, fn func(*rpc.Client) error) error {
	dialer := rpc.NewWebsocketDialer(rpc.DefaultWebsocketDialerConfig)
	client := rpc.NewClient(dialer)
	rpcCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := client.Start(rpcCtx, wsURL, func(error) {}); err != nil {
		return fmt.Errorf("failed to connect rpc client: %w", err)
	}

	if err := fn(client); err != nil {
		return err
	}
	return nil
}
