package service

import (
	"testing"

	"github.com/layer-3/nitrolite/pkg/core"
)

func TestBuildChannelGuidanceCheckpointableTransition(t *testing.T) {
	t.Parallel()

	overview := &DemoOverview{
		SelectedAsset: "yusd",
		Channel: &core.Channel{
			StateVersion: 4,
		},
		LatestState: &core.State{
			Version: 5,
			Transition: core.Transition{
				Type: core.TransitionTypeHomeDeposit,
			},
		},
	}

	got := buildChannelGuidance(overview)
	if got.NextAction != "checkpoint" {
		t.Fatalf("next action = %q, want checkpoint", got.NextAction)
	}
	if !got.SyncPending {
		t.Fatal("expected sync pending guidance")
	}
}

func TestBuildChannelGuidanceCommitTransitionWaitsForSync(t *testing.T) {
	t.Parallel()

	overview := &DemoOverview{
		SelectedAsset: "yusd",
		Channel: &core.Channel{
			StateVersion: 4,
		},
		LatestState: &core.State{
			Version: 6,
			Transition: core.Transition{
				Type: core.TransitionTypeCommit,
			},
		},
	}

	got := buildChannelGuidance(overview)
	if got.NextAction != "wait_for_sync" {
		t.Fatalf("next action = %q, want wait_for_sync", got.NextAction)
	}
	if !got.SyncPending {
		t.Fatal("expected sync pending guidance")
	}
}
