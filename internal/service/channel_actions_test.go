package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/layer-3/nitrolite-go-example/internal/nitrolite"
	"github.com/layer-3/nitrolite-go-example/internal/testsupport"
	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/shopspring/decimal"
)

func TestMutationService_Actions(t *testing.T) {
	t.Parallel()

	var approvedAsset string
	var depositedAmount decimal.Decimal
	var transferredRecipient string

	client := &testsupport.FakeClient{
		GetUserAddressFunc: func() string { return serviceTestAddress },
		ApproveTokenFunc: func(ctx context.Context, chainID uint64, asset string, amount decimal.Decimal) (string, error) {
			approvedAsset = asset
			return "0xapprove", nil
		},
		DepositFunc: func(ctx context.Context, blockchainID uint64, asset string, amount decimal.Decimal) (*core.State, error) {
			depositedAmount = amount
			return &core.State{ID: "0xdeposit", Asset: asset, Version: 2}, nil
		},
		WithdrawFunc: func(ctx context.Context, blockchainID uint64, asset string, amount decimal.Decimal) (*core.State, error) {
			return &core.State{ID: "0xwithdraw", Asset: asset, Version: 3}, nil
		},
		TransferFunc: func(ctx context.Context, recipientWallet string, asset string, amount decimal.Decimal) (*core.State, error) {
			transferredRecipient = recipientWallet
			return &core.State{ID: "0xtransfer", Asset: asset, Version: 4}, nil
		},
		CheckpointFunc: func(ctx context.Context, asset string) (string, error) {
			return "0xcheckpoint", nil
		},
		CloseHomeChannelFunc: func(ctx context.Context, asset string) (*core.State, error) {
			return &core.State{ID: "0xclose", Asset: asset, Version: 5}, nil
		},
		GetLatestStateFunc: func(ctx context.Context, wallet string, asset string, onlySigned bool) (*core.State, error) {
			return &core.State{ID: "0xstate", Asset: asset, Version: 6}, nil
		},
		ChallengeFunc: func(ctx context.Context, state core.State) (string, error) {
			return "0xchallenge", nil
		},
	}

	manager := nitrolite.NewManagerWithClient(client, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: serviceTestAddress,
	}, slog.Default())

	mutations := NewMutationService(manager, map[string]uint64{"usdc": 80002})
	channel := NewChannelService(manager)

	txHash, err := mutations.ApproveToken(context.Background(), 80002, "USDC", decimal.RequireFromString("100"))
	if err != nil {
		t.Fatalf("ApproveToken() error = %v", err)
	}
	if txHash != "0xapprove" || approvedAsset != "usdc" {
		t.Fatalf("approve result = %q, asset = %q", txHash, approvedAsset)
	}

	state, err := mutations.Deposit(context.Background(), 80002, "usdc", decimal.RequireFromString("5"))
	if err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	if state.ID != "0xdeposit" || depositedAmount.String() != "5" {
		t.Fatalf("deposit state = %#v, amount = %s", state, depositedAmount)
	}

	if _, err := mutations.Withdraw(context.Background(), 80002, "usdc", decimal.RequireFromString("2")); err != nil {
		t.Fatalf("Withdraw() error = %v", err)
	}

	if _, err := mutations.Transfer(context.Background(), "0x1111111111111111111111111111111111111111", "usdc", decimal.RequireFromString("1")); err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if transferredRecipient != "0x1111111111111111111111111111111111111111" {
		t.Fatalf("transferredRecipient = %q", transferredRecipient)
	}

	checkpointHash, err := mutations.Checkpoint(context.Background(), "usdc")
	if err != nil {
		t.Fatalf("Checkpoint() error = %v", err)
	}
	if checkpointHash != "0xcheckpoint" {
		t.Fatalf("checkpointHash = %q", checkpointHash)
	}

	closedState, err := channel.CloseChannel(context.Background(), CloseChannelRequest{Asset: "usdc"})
	if err != nil {
		t.Fatalf("CloseChannel() error = %v", err)
	}
	if closedState.ID != "0xclose" {
		t.Fatalf("closedState = %#v", closedState)
	}

	challengeResult, err := channel.ChallengeLatestState(context.Background(), ChallengeRequest{Asset: "usdc"})
	if err != nil {
		t.Fatalf("ChallengeLatestState() error = %v", err)
	}
	if challengeResult.TxHash != "0xchallenge" {
		t.Fatalf("challengeResult = %#v", challengeResult)
	}
}

func TestMutationService_Validation(t *testing.T) {
	t.Parallel()

	manager := nitrolite.NewManagerWithClient(&testsupport.FakeClient{
		GetUserAddressFunc: func() string { return serviceTestAddress },
	}, nitrolite.Health{
		Connected:     true,
		Ready:         true,
		SignerAddress: serviceTestAddress,
	}, slog.Default())

	mutations := NewMutationService(manager, map[string]uint64{"usdc": 80002})

	if _, err := mutations.Deposit(context.Background(), 0, "usdc", decimal.RequireFromString("1")); err == nil {
		t.Fatal("Deposit() error = nil, want validation error")
	}
	if _, err := mutations.Deposit(context.Background(), 11155111, "usdc", decimal.RequireFromString("1")); err == nil {
		t.Fatal("Deposit() mismatched chain error = nil, want validation error")
	}
	if _, err := mutations.Transfer(context.Background(), "not-an-address", "usdc", decimal.RequireFromString("1")); err == nil {
		t.Fatal("Transfer() error = nil, want validation error")
	}
	if _, err := mutations.Checkpoint(context.Background(), ""); err == nil {
		t.Fatal("Checkpoint() error = nil, want validation error")
	}
}
