package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
)

func TestSessionKeyHelpers(t *testing.T) {
	t.Parallel()

	assets, err := normalizeAssets([]string{"weth", "usdc", "weth"})
	if err != nil {
		t.Fatalf("normalizeAssets() error = %v", err)
	}
	if len(assets) != 2 || assets[0] != "usdc" || assets[1] != "weth" {
		t.Fatalf("assets = %#v", assets)
	}

	if got := nextChannelSessionKeyVersion([]core.ChannelSessionKeyStateV1{
		{SessionKey: "0x1", Version: 1},
		{SessionKey: "0x1", Version: 3},
	}, "0x1"); got != 4 {
		t.Fatalf("nextChannelSessionKeyVersion() = %d, want 4", got)
	}
	if got := nextAppSessionKeyVersion([]app.AppSessionKeyStateV1{
		{SessionKey: "0x1", Version: 2},
	}, "0x1"); got != 3 {
		t.Fatalf("nextAppSessionKeyVersion() = %d, want 3", got)
	}
}

func TestSessionKeyService_RegisterAndList(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	sessionKey := "0x1111111111111111111111111111111111111111"
	client := &testsupport.FakeClient{
		GetUserAddressFunc: func() string { return serviceTestAddress },
		GetLastChannelKeyStatesFunc: func(ctx context.Context, userAddress string, opts *sdk.GetLastChannelKeyStatesOptions) ([]core.ChannelSessionKeyStateV1, error) {
			return []core.ChannelSessionKeyStateV1{{SessionKey: sessionKey, Version: 1}}, nil
		},
		SignChannelSessionKeyStateFunc: func(state core.ChannelSessionKeyStateV1) (string, error) {
			return "0xchannelsig", nil
		},
		SubmitChannelSessionKeyStateFunc: func(ctx context.Context, state core.ChannelSessionKeyStateV1) error {
			return nil
		},
		GetLastAppKeyStatesFunc: func(ctx context.Context, userAddress string, opts *sdk.GetLastKeyStatesOptions) ([]app.AppSessionKeyStateV1, error) {
			return []app.AppSessionKeyStateV1{{SessionKey: sessionKey, Version: 2}}, nil
		},
		SignSessionKeyStateFunc: func(state app.AppSessionKeyStateV1) (string, error) {
			return "0xappsig", nil
		},
		SubmitAppSessionKeyStateFunc: func(ctx context.Context, state app.AppSessionKeyStateV1) error {
			return nil
		},
	}
	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: serviceTestAddress,
	}, slog.Default())
	svc := NewSessionKeyService(manager, func() time.Time { return now })

	channelState, err := svc.RegisterChannelKey(context.Background(), RegisterChannelSessionKeyRequest{
		SessionKey: sessionKey,
		Assets:     []string{"weth", "usdc", "weth"},
		ExpiresAt:  now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("RegisterChannelKey() error = %v", err)
	}
	if channelState.Version != 2 || channelState.UserSig != "0xchannelsig" {
		t.Fatalf("channelState = %#v", channelState)
	}

	appState, err := svc.RegisterAppKey(context.Background(), RegisterAppSessionKeyRequest{
		SessionKey:     sessionKey,
		ApplicationIDs: []string{"demo-app", "demo-app"},
		ExpiresAt:      now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("RegisterAppKey() error = %v", err)
	}
	if appState.Version != 3 || appState.UserSig != "0xappsig" {
		t.Fatalf("appState = %#v", appState)
	}
}
