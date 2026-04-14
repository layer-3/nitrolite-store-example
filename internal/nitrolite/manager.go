package nitrolite

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	appsigning "github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
)

// Health captures current client lifecycle state.
type Health struct {
	Connected     bool
	Ready         bool
	SignerAddress string
}

// Client captures the SDK methods currently used by the example app.
type Client interface {
	Close() error
	GetUserAddress() string
	Ping(ctx context.Context) error
	SetHomeBlockchain(asset string, blockchainID uint64) error
	WaitCh() <-chan struct{}
	GetConfig(ctx context.Context) (*core.NodeConfig, error)
	GetBlockchains(ctx context.Context) ([]core.Blockchain, error)
	GetAssets(ctx context.Context, blockchainID *uint64) ([]core.Asset, error)
	GetBalances(ctx context.Context, wallet string) ([]core.BalanceEntry, error)
	GetTransactions(ctx context.Context, wallet string, opts *sdk.GetTransactionsOptions) ([]core.Transaction, core.PaginationMetadata, error)
	GetHomeChannel(ctx context.Context, wallet, asset string) (*core.Channel, error)
	GetLatestState(ctx context.Context, wallet, asset string, onlySigned bool) (*core.State, error)
}

// Manager is a temporary lifecycle placeholder until the SDK wiring lands.
type Manager struct {
	mu     sync.RWMutex
	health Health
	client Client
	logger *slog.Logger
}

// NewManager constructs a placeholder manager for tests and bootstrap paths.
func NewManager(signerAddress string) *Manager {
	return &Manager{
		health: Health{
			Connected:     false,
			Ready:         false,
			SignerAddress: signerAddress,
		},
	}
}

// NewManagerWithClient constructs a manager around an injected client.
func NewManagerWithClient(client Client, health Health, logger *slog.Logger) *Manager {
	return &Manager{
		health: health,
		client: client,
		logger: logger,
	}
}

// NewSDKManager constructs a real SDK-backed manager.
func NewSDKManager(ctx context.Context, cfg *config.Config, signer appsigning.Signer, logger *slog.Logger) (*Manager, error) {
	opts := make([]sdk.Option, 0, len(cfg.BlockchainRPCURLs))
	for chainID, rpcURL := range cfg.BlockchainRPCURLs {
		parsed, err := strconv.ParseUint(chainID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid blockchain rpc chain id %q: %w", chainID, err)
		}
		opts = append(opts, sdk.WithBlockchainRPC(parsed, rpcURL))
	}

	sdkClient, err := sdk.NewClient(cfg.ClearnodeWSURL, signer.StateSigner(), signer.TxSigner(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create sdk client: %w", err)
	}

	for asset, chainID := range cfg.HomeBlockchains {
		if err := sdkClient.SetHomeBlockchain(asset, chainID); err != nil {
			_ = sdkClient.Close()
			return nil, fmt.Errorf("failed to set home blockchain for %s: %w", asset, err)
		}
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sdkClient.Ping(pingCtx); err != nil {
		_ = sdkClient.Close()
		return nil, fmt.Errorf("failed to ping clearnode: %w", err)
	}

	return &Manager{
		client: sdkClient,
		logger: logger,
		health: Health{
			Connected:     true,
			Ready:         true,
			SignerAddress: sdkClient.GetUserAddress(),
		},
	}, nil
}

// Health returns a copy of the current lifecycle state.
func (m *Manager) Health() Health {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health
}

// SetHealth replaces lifecycle state.
func (m *Manager) SetHealth(h Health) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.health = h
}

// Client returns the current SDK client.
func (m *Manager) Client() Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client
}

// Run marks the manager disconnected when the SDK client closes.
func (m *Manager) Run(ctx context.Context) {
	client := m.Client()
	if client == nil {
		<-ctx.Done()
		return
	}

	signerAddress := m.Health().SignerAddress

	select {
	case <-ctx.Done():
		_ = client.Close()
	case <-client.WaitCh():
		m.SetHealth(Health{
			Connected:     false,
			Ready:         false,
			SignerAddress: signerAddress,
		})
		if m.logger != nil {
			m.logger.Warn("sdk client closed")
		}
	}
}
