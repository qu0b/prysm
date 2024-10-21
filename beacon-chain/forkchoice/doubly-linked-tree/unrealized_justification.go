package doublylinkedtree

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/epoch/precompute"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"

	"github.com/antithesishq/antithesis-sdk-go/assert"
)

func (s *Store) setUnrealizedJustifiedEpoch(root [32]byte, epoch primitives.Epoch) error {
	node, ok := s.nodeByRoot[root]
	if !ok || node == nil {
		// Assert that node should not be nil
		assert.Unreachable("setUnrealizedJustifiedEpoch: Node not found", map[string]any{
			"root": root,
		})
		return errors.Wrap(ErrNilNode, "could not set unrealized justified epoch")
	}
	if epoch < node.unrealizedJustifiedEpoch {
		// Assert that epoch should not decrease
		assert.AlwaysGreaterThanOrEqualTo(
			epoch,
			node.unrealizedJustifiedEpoch,
			"setUnrealizedJustifiedEpoch: New epoch should not be less than current unrealizedJustifiedEpoch",
			map[string]any{
				"epoch": epoch,
				"unrealizedJustifiedEpoch": node.unrealizedJustifiedEpoch,
				"root": root,
			},
		)
		return errInvalidUnrealizedJustifiedEpoch
	}
	node.unrealizedJustifiedEpoch = epoch
	return nil
}

func (s *Store) setUnrealizedFinalizedEpoch(root [32]byte, epoch primitives.Epoch) error {
	node, ok := s.nodeByRoot[root]
	if !ok || node == nil {
		// Assert that node should not be nil
		assert.Unreachable("setUnrealizedFinalizedEpoch: Node not found", map[string]any{
			"root": root,
		})
		return errors.Wrap(ErrNilNode, "could not set unrealized finalized epoch")
	}
	if epoch < node.unrealizedFinalizedEpoch {
		// Assert that epoch should not decrease
		assert.AlwaysGreaterThanOrEqualTo(
			epoch,
			node.unrealizedFinalizedEpoch,
			"setUnrealizedFinalizedEpoch: New epoch should not be less than current unrealizedFinalizedEpoch",
			map[string]any{
				"epoch": epoch,
				"unrealizedFinalizedEpoch": node.unrealizedFinalizedEpoch,
				"root": root,
			},
		)
		return errInvalidUnrealizedFinalizedEpoch
	}
	node.unrealizedFinalizedEpoch = epoch
	return nil
}

// updateUnrealizedCheckpoints "realizes" the unrealized justified and finalized
// epochs stored within nodes. It should be called at the beginning of each epoch.
func (f *ForkChoice) updateUnrealizedCheckpoints(ctx context.Context) error {
	for _, node := range f.store.nodeByRoot {
		node.justifiedEpoch = node.unrealizedJustifiedEpoch
		node.finalizedEpoch = node.unrealizedFinalizedEpoch

		// Assert that node's justifiedEpoch is >= finalizedEpoch
		assert.AlwaysGreaterThanOrEqualTo(
			node.justifiedEpoch,
			node.finalizedEpoch,
			"updateUnrealizedCheckpoints: Node's justifiedEpoch should be >= finalizedEpoch",
			map[string]any{
				"nodeJustifiedEpoch": node.justifiedEpoch,
				"nodeFinalizedEpoch": node.finalizedEpoch,
			},
		)

		if node.justifiedEpoch > f.store.justifiedCheckpoint.Epoch {
			f.store.prevJustifiedCheckpoint = f.store.justifiedCheckpoint
			f.store.justifiedCheckpoint = f.store.unrealizedJustifiedCheckpoint

			// Assert that justified checkpoint epoch increases
			assert.AlwaysGreaterThan(
				f.store.justifiedCheckpoint.Epoch,
				f.store.prevJustifiedCheckpoint.Epoch,
				"updateUnrealizedCheckpoints: Justified checkpoint epoch should increase",
				map[string]any{
					"newJustifiedEpoch": f.store.justifiedCheckpoint.Epoch,
					"prevJustifiedEpoch": f.store.prevJustifiedCheckpoint.Epoch,
				},
			)

			if err := f.updateJustifiedBalances(ctx, f.store.justifiedCheckpoint.Root); err != nil {
				// Assert that updating justified balances should not fail
				assert.Unreachable("updateUnrealizedCheckpoints: Failed to update justified balances", map[string]any{
					"justifiedCheckpointRoot": f.store.justifiedCheckpoint.Root,
					"error": err,
				})
				return errors.Wrap(err, "could not update justified balances")
			}
		}
		if node.finalizedEpoch > f.store.finalizedCheckpoint.Epoch {
			f.store.finalizedCheckpoint = f.store.unrealizedFinalizedCheckpoint

			// Assert that finalized checkpoint epoch increases
			assert.AlwaysGreaterThan(
				f.store.finalizedCheckpoint.Epoch,
				f.store.prevFinalizedCheckpoint.Epoch,
				"updateUnrealizedCheckpoints: Finalized checkpoint epoch should increase",
				map[string]any{
					"newFinalizedEpoch": f.store.finalizedCheckpoint.Epoch,
					"prevFinalizedEpoch": f.store.prevFinalizedCheckpoint.Epoch,
				},
			)
		}
	}
	// Assert that store's justified checkpoint epoch >= finalized checkpoint epoch
	assert.AlwaysGreaterThanOrEqualTo(
		f.store.justifiedCheckpoint.Epoch,
		f.store.finalizedCheckpoint.Epoch,
		"updateUnrealizedCheckpoints: Store's justified checkpoint epoch should be >= finalized checkpoint epoch",
		map[string]any{
			"justifiedCheckpointEpoch": f.store.justifiedCheckpoint.Epoch,
			"finalizedCheckpointEpoch": f.store.finalizedCheckpoint.Epoch,
		},
	)
	return nil
}

func (s *Store) pullTips(state state.BeaconState, node *Node, jc, fc *ethpb.Checkpoint) (*ethpb.Checkpoint, *ethpb.Checkpoint) {
	if node.parent == nil { // Nothing to do if the parent is nil.
		return jc, fc
	}
	currentEpoch := slots.ToEpoch(slots.CurrentSlot(s.genesisTime))
	stateSlot := state.Slot()
	stateEpoch := slots.ToEpoch(stateSlot)
	currJustified := node.parent.unrealizedJustifiedEpoch == currentEpoch
	prevJustified := node.parent.unrealizedJustifiedEpoch+1 == currentEpoch
	tooEarlyForCurr := slots.SinceEpochStarts(stateSlot)*3 < params.BeaconConfig().SlotsPerEpoch*2
	// Exit early if it's justified or too early to be justified.
	if currJustified || (stateEpoch == currentEpoch && prevJustified && tooEarlyForCurr) {
		node.unrealizedJustifiedEpoch = node.parent.unrealizedJustifiedEpoch
		node.unrealizedFinalizedEpoch = node.parent.unrealizedFinalizedEpoch
		return jc, fc
	}

	uj, uf, err := precompute.UnrealizedCheckpoints(state)
	if err != nil {
		log.WithError(err).Debug("could not compute unrealized checkpoints")
		uj, uf = jc, fc

		// Assert that UnrealizedCheckpoints should not fail
		assert.Unreachable("pullTips: Failed to compute unrealized checkpoints", map[string]any{
			"stateSlot": stateSlot,
			"error": err,
		})
	}

	// Update store's unrealized checkpoints.
	if uj.Epoch > s.unrealizedJustifiedCheckpoint.Epoch {
		s.unrealizedJustifiedCheckpoint = &forkchoicetypes.Checkpoint{
			Epoch: uj.Epoch, Root: bytesutil.ToBytes32(uj.Root),
		}
	}
	if uf.Epoch > s.unrealizedFinalizedCheckpoint.Epoch {
		s.unrealizedJustifiedCheckpoint = &forkchoicetypes.Checkpoint{
			Epoch: uj.Epoch, Root: bytesutil.ToBytes32(uj.Root),
		}
		s.unrealizedFinalizedCheckpoint = &forkchoicetypes.Checkpoint{
			Epoch: uf.Epoch, Root: bytesutil.ToBytes32(uf.Root),
		}
	}

	// Update node's checkpoints.
	node.unrealizedJustifiedEpoch, node.unrealizedFinalizedEpoch = uj.Epoch, uf.Epoch

	// Assert that node's unrealizedJustifiedEpoch >= parent's unrealizedJustifiedEpoch
	assert.AlwaysGreaterThanOrEqualTo(
		node.unrealizedJustifiedEpoch,
		node.parent.unrealizedJustifiedEpoch,
		"pullTips: Node's unrealizedJustifiedEpoch should be >= parent's unrealizedJustifiedEpoch",
		map[string]any{
			"nodeUnrealizedJustifiedEpoch": node.unrealizedJustifiedEpoch,
			"parentUnrealizedJustifiedEpoch": node.parent.unrealizedJustifiedEpoch,
		},
	)

	// Assert that node's unrealizedFinalizedEpoch >= parent's unrealizedFinalizedEpoch
	assert.AlwaysGreaterThanOrEqualTo(
		node.unrealizedFinalizedEpoch,
		node.parent.unrealizedFinalizedEpoch,
		"pullTips: Node's unrealizedFinalizedEpoch should be >= parent's unrealizedFinalizedEpoch",
		map[string]any{
			"nodeUnrealizedFinalizedEpoch": node.unrealizedFinalizedEpoch,
			"parentUnrealizedFinalizedEpoch": node.parent.unrealizedFinalizedEpoch,
		},
	)

	if stateEpoch < currentEpoch {
		jc, fc = uj, uf
		node.justifiedEpoch = uj.Epoch
		node.finalizedEpoch = uf.Epoch
	}
	return jc, fc
}