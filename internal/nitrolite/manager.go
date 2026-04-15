package nitrolite

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/layer-3/nitrolite-go-example/internal/config"
	appsigning "github.com/layer-3/nitrolite-go-example/internal/signing"
	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
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
	GetApps(ctx context.Context, opts *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error)
	RegisterApp(ctx context.Context, appID string, metadata string, creationApprovalNotRequired bool) error
	GetAppSessions(ctx context.Context, opts *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error)
	GetAppDefinition(ctx context.Context, appSessionID string) (*app.AppDefinitionV1, error)
	CreateAppSession(ctx context.Context, definition app.AppDefinitionV1, sessionData string, quorumSigs []string, opts ...sdk.CreateAppSessionOptions) (string, string, string, error)
	SubmitAppSessionDeposit(ctx context.Context, appStateUpdate app.AppStateUpdateV1, quorumSigs []string, asset string, depositAmount decimal.Decimal) (string, error)
	SubmitAppState(ctx context.Context, appStateUpdate app.AppStateUpdateV1, quorumSigs []string) error
	SubmitAppSessionKeyState(ctx context.Context, state app.AppSessionKeyStateV1) error
	GetLastAppKeyStates(ctx context.Context, userAddress string, opts *sdk.GetLastKeyStatesOptions) ([]app.AppSessionKeyStateV1, error)
	SignSessionKeyState(state app.AppSessionKeyStateV1) (string, error)
	SubmitChannelSessionKeyState(ctx context.Context, state core.ChannelSessionKeyStateV1) error
	GetLastChannelKeyStates(ctx context.Context, userAddress string, opts *sdk.GetLastChannelKeyStatesOptions) ([]core.ChannelSessionKeyStateV1, error)
	SignChannelSessionKeyState(state core.ChannelSessionKeyStateV1) (string, error)
	CloseHomeChannel(ctx context.Context, asset string) (*core.State, error)
	Challenge(ctx context.Context, state core.State) (string, error)
	ApproveToken(ctx context.Context, chainID uint64, asset string, amount decimal.Decimal) (string, error)
	Deposit(ctx context.Context, blockchainID uint64, asset string, amount decimal.Decimal) (*core.State, error)
	Withdraw(ctx context.Context, blockchainID uint64, asset string, amount decimal.Decimal) (*core.State, error)
	Transfer(ctx context.Context, recipientWallet string, asset string, amount decimal.Decimal) (*core.State, error)
	Checkpoint(ctx context.Context, asset string) (string, error)
}

// Manager is a temporary lifecycle placeholder until the SDK wiring lands.
type Manager struct {
	mu         sync.RWMutex
	health     Health
	client     Client
	logger     *slog.Logger
	reconnect  func(context.Context) (Client, error)
	sleep      func(context.Context, time.Duration) bool
	runStarted atomic.Bool
}

// NewManager constructs a placeholder manager for tests and bootstrap paths.
func NewManager(signerAddress string) *Manager {
	return &Manager{
		sleep: sleepWithContext,
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
		sleep:  sleepWithContext,
	}
}

// NewSDKManager constructs a real SDK-backed manager.
func NewSDKManager(ctx context.Context, cfg *config.Config, signer appsigning.Signer, logger *slog.Logger) (*Manager, error) {
	sdkClient, err := connectSDKClient(ctx, cfg, signer)
	if err != nil {
		return nil, err
	}

	return &Manager{
		client: sdkClient,
		logger: logger,
		sleep:  sleepWithContext,
		reconnect: func(ctx context.Context) (Client, error) {
			return connectSDKClient(ctx, cfg, signer)
		},
		health: Health{
			Connected:     true,
			Ready:         true,
			SignerAddress: sdkClient.GetUserAddress(),
		},
	}, nil
}

func connectSDKClient(ctx context.Context, cfg *config.Config, signer appsigning.Signer) (Client, error) {
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

	return sdkClient, nil
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
	if !m.runStarted.CompareAndSwap(false, true) {
		if m.logger != nil {
			m.logger.Warn("manager run already started")
		}
		return
	}

	signerAddress := m.Health().SignerAddress

	for {
		client := m.Client()
		if client == nil {
			<-ctx.Done()
			return
		}

		select {
		case <-ctx.Done():
			_ = client.Close()
			return
		case <-client.WaitCh():
			m.SetHealth(Health{Connected: false, Ready: false, SignerAddress: signerAddress})
			if m.logger != nil {
				m.logger.Warn("sdk client closed")
			}
			if !m.reconnectUntilConnected(ctx, signerAddress) {
				return
			}
		}
	}
}

func (m *Manager) reconnectUntilConnected(ctx context.Context, signerAddress string) bool {
	if m.reconnect == nil {
		return false
	}

	attempt := 0
	for {
		attempt++
		client, err := m.reconnect(ctx)
		if err == nil {
			m.mu.Lock()
			m.client = client
			m.health = Health{
				Connected:     true,
				Ready:         true,
				SignerAddress: signerAddress,
			}
			m.mu.Unlock()
			if m.logger != nil {
				m.logger.Info("sdk client reconnected", "attempt", attempt)
			}
			return true
		}

		if m.logger != nil && attempt >= 5 {
			m.logger.Error("reconnect failing", "attempt", attempt, "error", err)
		}

		if !m.sleep(ctx, reconnectBackoff(attempt)) {
			return false
		}
	}
}

func reconnectBackoff(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return 1 * time.Second
	case attempt == 2:
		return 2 * time.Second
	case attempt == 3:
		return 4 * time.Second
	case attempt == 4:
		return 8 * time.Second
	case attempt == 5:
		return 16 * time.Second
	default:
		return 30 * time.Second
	}
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
