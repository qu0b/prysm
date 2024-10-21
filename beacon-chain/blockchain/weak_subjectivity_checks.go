package blockchain

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filters"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"

	"github.com/antithesishq/antithesis-sdk-go/assert"
)

type weakSubjectivityDB interface {
	HasBlock(ctx context.Context, blockRoot [32]byte) bool
	BlockRoots(ctx context.Context, f *filters.QueryFilter) ([][32]byte, error)
}

type WeakSubjectivityVerifier struct {
	enabled  bool
	verified bool
	root     [32]byte
	epoch    primitives.Epoch
	slot     primitives.Slot
	db       weakSubjectivityDB
}

// NewWeakSubjectivityVerifier validates a checkpoint, and if valid, uses it to initialize a weak subjectivity verifier.
func NewWeakSubjectivityVerifier(wsc *ethpb.Checkpoint, db weakSubjectivityDB) (*WeakSubjectivityVerifier, error) {
	if wsc == nil || len(wsc.Root) == 0 || wsc.Epoch == 0 {
		log.Debug("--weak-subjectivity-checkpoint not provided")
		return &WeakSubjectivityVerifier{
			enabled: false,
		}, nil
	}
	startSlot, err := slots.EpochStart(wsc.Epoch)
	if err != nil {
		return nil, err
	}
	return &WeakSubjectivityVerifier{
		enabled:  true,
		verified: false,
		root:     bytesutil.ToBytes32(wsc.Root),
		epoch:    wsc.Epoch,
		db:       db,
		slot:     startSlot,
	}, nil
}

// VerifyWeakSubjectivity verifies the weak subjectivity root in the service struct.
// Reference design: https://github.com/ethereum/consensus-specs/blob/master/specs/phase0/weak-subjectivity.md#weak-subjectivity-sync-procedure
func (v *WeakSubjectivityVerifier) VerifyWeakSubjectivity(ctx context.Context, finalizedEpoch primitives.Epoch) error {
	if v.verified || !v.enabled {
		return nil
	}

	// Assert that v.db is not nil
	assert.Always(v.db != nil, "WeakSubjectivityVerifier db should not be nil", nil)

	// Zero value for [32]byte
	var zeroRoot [32]byte
	// Assert that v.root is not zero
	assert.Always(v.root != zeroRoot, "WeakSubjectivityVerifier root should not be zero", nil)

	// Assert that v.slot is valid (non-negative)
	assert.Always(v.slot >= 0, "WeakSubjectivityVerifier slot should be non-negative", map[string]any{
		"slot": v.slot,
	})

	// Two conditions are described in the specs:
	// IF epoch_number > store.finalized_checkpoint.epoch,
	// then ASSERT during block sync that block with root block_root
	// is in the sync path at epoch epoch_number. Emit descriptive critical error if this assert fails,
	// then exit client process.
	// we do not handle this case ^, because we can only blocks that have been processed / are currently
	// in line for finalization, we don't have the ability to look ahead. so we only satisfy the following:
	// IF epoch_number <= store.finalized_checkpoint.epoch,
	// then ASSERT that the block in the canonical chain at epoch epoch_number has root block_root.
	// Emit descriptive critical error if this assert fails, then exit client process.
	if v.epoch > finalizedEpoch {
		return nil
	}

	// Assert that v.epoch <= finalizedEpoch
	assert.Always(v.epoch <= finalizedEpoch, "WeakSubjectivityVerifier epoch should be less than or equal to finalizedEpoch", map[string]any{
		"v.epoch":        v.epoch,
		"finalizedEpoch": finalizedEpoch,
	})

	log.Infof("Performing weak subjectivity check for root %#x in epoch %d", v.root, v.epoch)

	// Assert that block exists in database
	hasBlock := v.db.HasBlock(ctx, v.root)
	assert.Always(hasBlock, "Weak subjectivity block must exist in database", map[string]any{
		"root": v.root,
	})

	if !hasBlock {
		return errors.Wrap(errWSBlockNotFound, fmt.Sprintf("missing root %#x", v.root))
	}

	endSlot := v.slot + params.BeaconConfig().SlotsPerEpoch

	// Assert that endSlot > v.slot
	assert.Always(endSlot > v.slot, "End slot should be greater than start slot", map[string]any{
		"startSlot": v.slot,
		"endSlot":   endSlot,
	})

	filter := filters.NewFilter().SetStartSlot(v.slot).SetEndSlot(endSlot)
	// A node should have the weak subjectivity block corresponds to the correct epoch in the DB.
	log.Infof("Searching block roots for weak subjectivity root=%#x, between slots %d-%d", v.root, v.slot, endSlot)
	roots, err := v.db.BlockRoots(ctx, filter)
	if err != nil {
		return errors.Wrap(err, "error while retrieving block roots to verify weak subjectivity")
	}

	// Assert that roots are retrieved
	assert.Always(len(roots) > 0, "Block roots should be retrieved for given slot range", map[string]any{
		"startSlot":      v.slot,
		"endSlot":        endSlot,
		"retrievedRoots": len(roots),
	})

	for _, root := range roots {
		if v.root == root {
			log.Info("Weak subjectivity check has passed!!")
			v.verified = true
			return nil
		}
	}
	return errors.Wrap(errWSBlockNotFoundInEpoch, fmt.Sprintf("root=%#x, epoch=%d", v.root, v.epoch))
}