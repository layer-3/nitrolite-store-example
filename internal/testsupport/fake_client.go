package testsupport

import (
	"context"

	"github.com/layer-3/nitrolite/pkg/app"
	"github.com/layer-3/nitrolite/pkg/core"
	sdk "github.com/layer-3/nitrolite/sdk/go"
	"github.com/shopspring/decimal"
)

// FakeClient is a configurable test double for nitrolite.Client.
type FakeClient struct {
	CloseFunc                     func() error
	GetUserAddressFunc            func() string
	PingFunc                      func(context.Context) error
	SetHomeBlockchainFunc         func(string, uint64) error
	WaitChFunc                    func() <-chan struct{}
	GetConfigFunc                 func(context.Context) (*core.NodeConfig, error)
	GetBlockchainsFunc            func(context.Context) ([]core.Blockchain, error)
	GetAssetsFunc                 func(context.Context, *uint64) ([]core.Asset, error)
	GetBalancesFunc               func(context.Context, string) ([]core.BalanceEntry, error)
	GetTransactionsFunc           func(context.Context, string, *sdk.GetTransactionsOptions) ([]core.Transaction, core.PaginationMetadata, error)
	GetHomeChannelFunc            func(context.Context, string, string) (*core.Channel, error)
	GetLatestStateFunc            func(context.Context, string, string, bool) (*core.State, error)
	GetAppsFunc                   func(context.Context, *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error)
	RegisterAppFunc               func(context.Context, string, string, bool) error
	GetAppSessionsFunc            func(context.Context, *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error)
	GetAppDefinitionFunc          func(context.Context, string) (*app.AppDefinitionV1, error)
	CreateAppSessionFunc          func(context.Context, app.AppDefinitionV1, string, []string, ...sdk.CreateAppSessionOptions) (string, string, string, error)
	SubmitAppSessionDepositFunc   func(context.Context, app.AppStateUpdateV1, []string, string, decimal.Decimal) (string, error)
	SubmitAppStateFunc            func(context.Context, app.AppStateUpdateV1, []string) error
	SubmitAppSessionKeyStateFunc  func(context.Context, app.AppSessionKeyStateV1) error
	GetLastAppKeyStatesFunc       func(context.Context, string, *sdk.GetLastKeyStatesOptions) ([]app.AppSessionKeyStateV1, error)
	SignSessionKeyStateFunc       func(app.AppSessionKeyStateV1) (string, error)
	SubmitChannelSessionKeyStateFunc func(context.Context, core.ChannelSessionKeyStateV1) error
	GetLastChannelKeyStatesFunc   func(context.Context, string, *sdk.GetLastChannelKeyStatesOptions) ([]core.ChannelSessionKeyStateV1, error)
	SignChannelSessionKeyStateFunc func(core.ChannelSessionKeyStateV1) (string, error)
	CloseHomeChannelFunc          func(context.Context, string) (*core.State, error)
	ChallengeFunc                 func(context.Context, core.State) (string, error)
	ApproveTokenFunc              func(context.Context, uint64, string, decimal.Decimal) (string, error)
	DepositFunc                   func(context.Context, uint64, string, decimal.Decimal) (*core.State, error)
	WithdrawFunc                  func(context.Context, uint64, string, decimal.Decimal) (*core.State, error)
	TransferFunc                  func(context.Context, string, string, decimal.Decimal) (*core.State, error)
	CheckpointFunc                func(context.Context, string) (string, error)
}

func (f *FakeClient) Close() error {
	if f.CloseFunc != nil {
		return f.CloseFunc()
	}
	return nil
}

func (f *FakeClient) GetUserAddress() string {
	if f.GetUserAddressFunc != nil {
		return f.GetUserAddressFunc()
	}
	return ""
}

func (f *FakeClient) Ping(ctx context.Context) error {
	if f.PingFunc != nil {
		return f.PingFunc(ctx)
	}
	return nil
}

func (f *FakeClient) SetHomeBlockchain(asset string, blockchainID uint64) error {
	if f.SetHomeBlockchainFunc != nil {
		return f.SetHomeBlockchainFunc(asset, blockchainID)
	}
	return nil
}

func (f *FakeClient) WaitCh() <-chan struct{} {
	if f.WaitChFunc != nil {
		return f.WaitChFunc()
	}
	ch := make(chan struct{})
	return ch
}

func (f *FakeClient) GetConfig(ctx context.Context) (*core.NodeConfig, error) {
	if f.GetConfigFunc != nil {
		return f.GetConfigFunc(ctx)
	}
	return nil, nil
}

func (f *FakeClient) GetBlockchains(ctx context.Context) ([]core.Blockchain, error) {
	if f.GetBlockchainsFunc != nil {
		return f.GetBlockchainsFunc(ctx)
	}
	return nil, nil
}

func (f *FakeClient) GetAssets(ctx context.Context, blockchainID *uint64) ([]core.Asset, error) {
	if f.GetAssetsFunc != nil {
		return f.GetAssetsFunc(ctx, blockchainID)
	}
	return nil, nil
}

func (f *FakeClient) GetBalances(ctx context.Context, wallet string) ([]core.BalanceEntry, error) {
	if f.GetBalancesFunc != nil {
		return f.GetBalancesFunc(ctx, wallet)
	}
	return nil, nil
}

func (f *FakeClient) GetTransactions(ctx context.Context, wallet string, opts *sdk.GetTransactionsOptions) ([]core.Transaction, core.PaginationMetadata, error) {
	if f.GetTransactionsFunc != nil {
		return f.GetTransactionsFunc(ctx, wallet, opts)
	}
	return nil, core.PaginationMetadata{}, nil
}

func (f *FakeClient) GetHomeChannel(ctx context.Context, wallet string, asset string) (*core.Channel, error) {
	if f.GetHomeChannelFunc != nil {
		return f.GetHomeChannelFunc(ctx, wallet, asset)
	}
	return nil, nil
}

func (f *FakeClient) GetLatestState(ctx context.Context, wallet string, asset string, onlySigned bool) (*core.State, error) {
	if f.GetLatestStateFunc != nil {
		return f.GetLatestStateFunc(ctx, wallet, asset, onlySigned)
	}
	return nil, nil
}

func (f *FakeClient) GetApps(ctx context.Context, opts *sdk.GetAppsOptions) ([]app.AppInfoV1, core.PaginationMetadata, error) {
	if f.GetAppsFunc != nil {
		return f.GetAppsFunc(ctx, opts)
	}
	return nil, core.PaginationMetadata{}, nil
}

func (f *FakeClient) RegisterApp(ctx context.Context, appID string, metadata string, creationApprovalNotRequired bool) error {
	if f.RegisterAppFunc != nil {
		return f.RegisterAppFunc(ctx, appID, metadata, creationApprovalNotRequired)
	}
	return nil
}

func (f *FakeClient) GetAppSessions(ctx context.Context, opts *sdk.GetAppSessionsOptions) ([]app.AppSessionInfoV1, core.PaginationMetadata, error) {
	if f.GetAppSessionsFunc != nil {
		return f.GetAppSessionsFunc(ctx, opts)
	}
	return nil, core.PaginationMetadata{}, nil
}

func (f *FakeClient) GetAppDefinition(ctx context.Context, appSessionID string) (*app.AppDefinitionV1, error) {
	if f.GetAppDefinitionFunc != nil {
		return f.GetAppDefinitionFunc(ctx, appSessionID)
	}
	return nil, nil
}

func (f *FakeClient) CreateAppSession(ctx context.Context, definition app.AppDefinitionV1, sessionData string, quorumSigs []string, opts ...sdk.CreateAppSessionOptions) (string, string, string, error) {
	if f.CreateAppSessionFunc != nil {
		return f.CreateAppSessionFunc(ctx, definition, sessionData, quorumSigs, opts...)
	}
	return "", "", "", nil
}

func (f *FakeClient) SubmitAppSessionDeposit(ctx context.Context, update app.AppStateUpdateV1, quorumSigs []string, asset string, amount decimal.Decimal) (string, error) {
	if f.SubmitAppSessionDepositFunc != nil {
		return f.SubmitAppSessionDepositFunc(ctx, update, quorumSigs, asset, amount)
	}
	return "", nil
}

func (f *FakeClient) SubmitAppState(ctx context.Context, update app.AppStateUpdateV1, quorumSigs []string) error {
	if f.SubmitAppStateFunc != nil {
		return f.SubmitAppStateFunc(ctx, update, quorumSigs)
	}
	return nil
}

func (f *FakeClient) SubmitAppSessionKeyState(ctx context.Context, state app.AppSessionKeyStateV1) error {
	if f.SubmitAppSessionKeyStateFunc != nil {
		return f.SubmitAppSessionKeyStateFunc(ctx, state)
	}
	return nil
}

func (f *FakeClient) GetLastAppKeyStates(ctx context.Context, userAddress string, opts *sdk.GetLastKeyStatesOptions) ([]app.AppSessionKeyStateV1, error) {
	if f.GetLastAppKeyStatesFunc != nil {
		return f.GetLastAppKeyStatesFunc(ctx, userAddress, opts)
	}
	return nil, nil
}

func (f *FakeClient) SignSessionKeyState(state app.AppSessionKeyStateV1) (string, error) {
	if f.SignSessionKeyStateFunc != nil {
		return f.SignSessionKeyStateFunc(state)
	}
	return "", nil
}

func (f *FakeClient) SubmitChannelSessionKeyState(ctx context.Context, state core.ChannelSessionKeyStateV1) error {
	if f.SubmitChannelSessionKeyStateFunc != nil {
		return f.SubmitChannelSessionKeyStateFunc(ctx, state)
	}
	return nil
}

func (f *FakeClient) GetLastChannelKeyStates(ctx context.Context, userAddress string, opts *sdk.GetLastChannelKeyStatesOptions) ([]core.ChannelSessionKeyStateV1, error) {
	if f.GetLastChannelKeyStatesFunc != nil {
		return f.GetLastChannelKeyStatesFunc(ctx, userAddress, opts)
	}
	return nil, nil
}

func (f *FakeClient) SignChannelSessionKeyState(state core.ChannelSessionKeyStateV1) (string, error) {
	if f.SignChannelSessionKeyStateFunc != nil {
		return f.SignChannelSessionKeyStateFunc(state)
	}
	return "", nil
}

func (f *FakeClient) CloseHomeChannel(ctx context.Context, asset string) (*core.State, error) {
	if f.CloseHomeChannelFunc != nil {
		return f.CloseHomeChannelFunc(ctx, asset)
	}
	return nil, nil
}

func (f *FakeClient) Challenge(ctx context.Context, state core.State) (string, error) {
	if f.ChallengeFunc != nil {
		return f.ChallengeFunc(ctx, state)
	}
	return "", nil
}

func (f *FakeClient) ApproveToken(ctx context.Context, chainID uint64, asset string, amount decimal.Decimal) (string, error) {
	if f.ApproveTokenFunc != nil {
		return f.ApproveTokenFunc(ctx, chainID, asset, amount)
	}
	return "", nil
}

func (f *FakeClient) Deposit(ctx context.Context, blockchainID uint64, asset string, amount decimal.Decimal) (*core.State, error) {
	if f.DepositFunc != nil {
		return f.DepositFunc(ctx, blockchainID, asset, amount)
	}
	return nil, nil
}

func (f *FakeClient) Withdraw(ctx context.Context, blockchainID uint64, asset string, amount decimal.Decimal) (*core.State, error) {
	if f.WithdrawFunc != nil {
		return f.WithdrawFunc(ctx, blockchainID, asset, amount)
	}
	return nil, nil
}

func (f *FakeClient) Transfer(ctx context.Context, recipientWallet string, asset string, amount decimal.Decimal) (*core.State, error) {
	if f.TransferFunc != nil {
		return f.TransferFunc(ctx, recipientWallet, asset, amount)
	}
	return nil, nil
}

func (f *FakeClient) Checkpoint(ctx context.Context, asset string) (string, error) {
	if f.CheckpointFunc != nil {
		return f.CheckpointFunc(ctx, asset)
	}
	return "", nil
}
