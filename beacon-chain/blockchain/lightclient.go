package blockchain

import (
	"bytes"
	"context"
	"fmt"

	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpbv1 "github.com/prysmaticlabs/prysm/v5/proto/eth/v1"
	ethpbv2 "github.com/prysmaticlabs/prysm/v5/proto/eth/v2"
	"github.com/prysmaticlabs/prysm/v5/proto/migration"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

const (
	finalityBranchNumOfLeaves = 6
)

// CreateLightClientFinalityUpdate - implements https://github.com/ethereum/consensus-specs/blob/3d235740e5f1e641d3b160c8688f26e7dc5a1894/specs/altair/light-client/full-node.md#create_light_client_finality_update
// def create_light_client_finality_update(update: LightClientUpdate) -> LightClientFinalityUpdate:
//
//	return LightClientFinalityUpdate(
//	    attested_header=update.attested_header,
//	    finalized_header=update.finalized_header,
//	    finality_branch=update.finality_branch,
//	    sync_aggregate=update.sync_aggregate,
//	    signature_slot=update.signature_slot,
//	)
func CreateLightClientFinalityUpdate(update *ethpbv2.LightClientUpdate) *ethpbv2.LightClientFinalityUpdate {
	return &ethpbv2.LightClientFinalityUpdate{
		AttestedHeader:  update.AttestedHeader,
		FinalizedHeader: update.FinalizedHeader,
		FinalityBranch:  update.FinalityBranch,
		SyncAggregate:   update.SyncAggregate,
		SignatureSlot:   update.SignatureSlot,
	}
}

// CreateLightClientOptimisticUpdate - implements https://github.com/ethereum/consensus-specs/blob/3d235740e5f1e641d3b160c8688f26e7dc5a1894/specs/altair/light-client/full-node.md#create_light_client_optimistic_update
// def create_light_client_optimistic_update(update: LightClientUpdate) -> LightClientOptimisticUpdate:
//
//	return LightClientOptimisticUpdate(
//	    attested_header=update.attested_header,
//	    sync_aggregate=update.sync_aggregate,
//	    signature_slot=update.signature_slot,
//	)
func CreateLightClientOptimisticUpdate(update *ethpbv2.LightClientUpdate) *ethpbv2.LightClientOptimisticUpdate {
	return &ethpbv2.LightClientOptimisticUpdate{
		AttestedHeader: update.AttestedHeader,
		SyncAggregate:  update.SyncAggregate,
		SignatureSlot:  update.SignatureSlot,
	}
}

func NewLightClientOptimisticUpdateFromBeaconState(
	ctx context.Context,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState) (*ethpbv2.LightClientUpdate, error) {
	// assert compute_epoch_at_slot(attested_state.slot) >= ALTAIR_FORK_EPOCH
	attestedEpoch := slots.ToEpoch(attestedState.Slot())
	assert.AlwaysGreaterThanOrEqualTo(attestedEpoch, params.BeaconConfig().AltairForkEpoch, "Attested epoch should be >= ALTAIR_FORK_EPOCH", map[string]any{
		"attestedEpoch":     attestedEpoch,
		"ALTAIR_FORK_EPOCH": params.BeaconConfig().AltairForkEpoch,
	})
	if attestedEpoch < params.BeaconConfig().AltairForkEpoch {
		return nil, fmt.Errorf("invalid attested epoch %d", attestedEpoch)
	}

	// assert sum(block.message.body.sync_aggregate.sync_committee_bits) >= MIN_SYNC_COMMITTEE_PARTICIPANTS
	syncAggregate, err := block.Block().Body().SyncAggregate()
	if err != nil {
		return nil, fmt.Errorf("could not get sync aggregate %v", err)
	}
	assert.AlwaysGreaterThanOrEqualTo(syncAggregate.SyncCommitteeBits.Count(), params.BeaconConfig().MinSyncCommitteeParticipants, "Sync committee bits count should be >= MIN_SYNC_COMMITTEE_PARTICIPANTS", map[string]any{
		"syncCommitteeBitsCount":       syncAggregate.SyncCommitteeBits.Count(),
		"MinSyncCommitteeParticipants": params.BeaconConfig().MinSyncCommitteeParticipants,
	})
	if syncAggregate.SyncCommitteeBits.Count() < params.BeaconConfig().MinSyncCommitteeParticipants {
		return nil, fmt.Errorf("invalid sync committee bits count %d", syncAggregate.SyncCommitteeBits.Count())
	}

	// assert state.slot == state.latest_block_header.slot
	assert.Always(state.Slot() == state.LatestBlockHeader().Slot, "State slot should equal latest block header slot", map[string]any{
		"stateSlot":             state.Slot(),
		"latestBlockHeaderSlot": state.LatestBlockHeader().Slot,
	})
	if state.Slot() != state.LatestBlockHeader().Slot {
		return nil, fmt.Errorf("state slot %d not equal to latest block header slot %d", state.Slot(), state.LatestBlockHeader().Slot)
	}

	// assert hash_tree_root(header) == hash_tree_root(block.message)
	header := state.LatestBlockHeader()
	stateRoot, err := state.HashTreeRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get state root %v", err)
	}
	header.StateRoot = stateRoot[:]

	headerRoot, err := header.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("could not get header root %v", err)
	}

	blockRoot, err := block.Block().HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("could not get block root %v", err)
	}
	assert.Always(bytes.Equal(headerRoot[:], blockRoot[:]), "Header root should equal block root", map[string]any{
		"headerRoot": headerRoot,
		"blockRoot":  blockRoot,
	})
	if headerRoot != blockRoot {
		return nil, fmt.Errorf("header root %#x not equal to block root %#x", headerRoot, blockRoot)
	}

	// assert attested_state.slot == attested_state.latest_block_header.slot
	assert.Always(attestedState.Slot() == attestedState.LatestBlockHeader().Slot, "Attested state slot should equal latest block header slot", map[string]any{
		"attestedStateSlot":             attestedState.Slot(),
		"attestedLatestBlockHeaderSlot": attestedState.LatestBlockHeader().Slot,
	})
	if attestedState.Slot() != attestedState.LatestBlockHeader().Slot {
		return nil, fmt.Errorf("attested state slot %d not equal to attested latest block header slot %d", attestedState.Slot(), attestedState.LatestBlockHeader().Slot)
	}

	// attested_header = attested_state.latest_block_header.copy()
	attestedHeader := attestedState.LatestBlockHeader()

	// attested_header.state_root = hash_tree_root(attested_state)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get attested state root %v", err)
	}
	attestedHeader.StateRoot = attestedStateRoot[:]

	// assert hash_tree_root(attested_header) == block.message.parent_root
	attestedHeaderRoot, err := attestedHeader.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("could not get attested header root %v", err)
	}
	parentRoot := block.Block().ParentRoot()
	assert.Always(bytes.Equal(attestedHeaderRoot[:], parentRoot[:]), "Attested header root should equal block parent root", map[string]any{
		"attestedHeaderRoot": attestedHeaderRoot,
		"blockParentRoot":    block.Block().ParentRoot(),
	})
	if attestedHeaderRoot != block.Block().ParentRoot() {
		return nil, fmt.Errorf("attested header root %#x not equal to block parent root %#x", attestedHeaderRoot, block.Block().ParentRoot())
	}

	// Return result
	attestedHeaderResult := &ethpbv1.BeaconBlockHeader{
		Slot:          attestedHeader.Slot,
		ProposerIndex: attestedHeader.ProposerIndex,
		ParentRoot:    attestedHeader.ParentRoot,
		StateRoot:     attestedHeader.StateRoot,
		BodyRoot:      attestedHeader.BodyRoot,
	}

	syncAggregateResult := &ethpbv1.SyncAggregate{
		SyncCommitteeBits:      syncAggregate.SyncCommitteeBits,
		SyncCommitteeSignature: syncAggregate.SyncCommitteeSignature,
	}

	result := &ethpbv2.LightClientUpdate{
		AttestedHeader: attestedHeaderResult,
		SyncAggregate:  syncAggregateResult,
		SignatureSlot:  block.Block().Slot(),
	}

	return result, nil
}

func NewLightClientFinalityUpdateFromBeaconState(
	ctx context.Context,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState,
	finalizedBlock interfaces.ReadOnlySignedBeaconBlock) (*ethpbv2.LightClientUpdate, error) {
	result, err := NewLightClientOptimisticUpdateFromBeaconState(
		ctx,
		state,
		block,
		attestedState,
	)
	if err != nil {
		return nil, err
	}

	// Indicate finality whenever possible
	var finalizedHeader *ethpbv1.BeaconBlockHeader
	var finalityBranch [][]byte

	if finalizedBlock != nil && !finalizedBlock.IsNil() {
		if finalizedBlock.Block().Slot() != 0 {
			tempFinalizedHeader, err := finalizedBlock.Header()
			if err != nil {
				return nil, fmt.Errorf("could not get finalized header %v", err)
			}
			finalizedHeader = migration.V1Alpha1SignedHeaderToV1(tempFinalizedHeader).GetMessage()

			finalizedHeaderRoot, err := finalizedHeader.HashTreeRoot()
			if err != nil {
				return nil, fmt.Errorf("could not get finalized header root %v", err)
			}
			attestcpRoot := bytesutil.ToBytes32(attestedState.FinalizedCheckpoint().Root)
			assert.Always(bytes.Equal(finalizedHeaderRoot[:], attestcpRoot[:]), "Finalized header root should equal attested finalized checkpoint root", map[string]any{
				"finalizedHeaderRoot":             finalizedHeaderRoot,
				"attestedFinalizedCheckpointRoot": attestedState.FinalizedCheckpoint().Root,
			})
			if finalizedHeaderRoot != bytesutil.ToBytes32(attestedState.FinalizedCheckpoint().Root) {
				return nil, fmt.Errorf("finalized header root %#x not equal to attested finalized checkpoint root %#x", finalizedHeaderRoot, bytesutil.ToBytes32(attestedState.FinalizedCheckpoint().Root))
			}
		} else {
			assert.Always(bytes.Equal(attestedState.FinalizedCheckpoint().Root, make([]byte, 32)), "When finalized block slot is zero, attested finalized checkpoint root should be zeroed", map[string]any{
				"attestedFinalizedCheckpointRoot": attestedState.FinalizedCheckpoint().Root,
			})
			if !bytes.Equal(attestedState.FinalizedCheckpoint().Root, make([]byte, 32)) {
				return nil, fmt.Errorf("invalid finalized header root %v", attestedState.FinalizedCheckpoint().Root)
			}

			finalizedHeader = &ethpbv1.BeaconBlockHeader{
				Slot:          0,
				ProposerIndex: 0,
				ParentRoot:    make([]byte, 32),
				StateRoot:     make([]byte, 32),
				BodyRoot:      make([]byte, 32),
			}
		}

		var bErr error
		finalityBranch, bErr = attestedState.FinalizedRootProof(ctx)
		if bErr != nil {
			return nil, fmt.Errorf("could not get finalized root proof %v", bErr)
		}
	} else {
		finalizedHeader = &ethpbv1.BeaconBlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    make([]byte, 32),
			StateRoot:     make([]byte, 32),
			BodyRoot:      make([]byte, 32),
		}

		finalityBranch = make([][]byte, finalityBranchNumOfLeaves)
		for i := 0; i < finalityBranchNumOfLeaves; i++ {
			finalityBranch[i] = make([]byte, 32)
		}
	}

	result.FinalizedHeader = finalizedHeader
	result.FinalityBranch = finalityBranch
	return result, nil
}

func NewLightClientUpdateFromFinalityUpdate(update *ethpbv2.LightClientFinalityUpdate) *ethpbv2.LightClientUpdate {
	return &ethpbv2.LightClientUpdate{
		AttestedHeader:  update.AttestedHeader,
		FinalizedHeader: update.FinalizedHeader,
		FinalityBranch:  update.FinalityBranch,
		SyncAggregate:   update.SyncAggregate,
		SignatureSlot:   update.SignatureSlot,
	}
}

func NewLightClientUpdateFromOptimisticUpdate(update *ethpbv2.LightClientOptimisticUpdate) *ethpbv2.LightClientUpdate {
	return &ethpbv2.LightClientUpdate{
		AttestedHeader: update.AttestedHeader,
		SyncAggregate:  update.SyncAggregate,
		SignatureSlot:  update.SignatureSlot,
	}
}
