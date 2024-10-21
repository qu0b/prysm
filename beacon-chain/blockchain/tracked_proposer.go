package blockchain

import (
	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// trackedProposer returns whether the beacon node was informed, via the
// validators/prepare_proposer endpoint, of the proposer at the given slot.
// It only returns true if the tracked proposer is present and active.
func (s *Service) trackedProposer(st state.ReadOnlyBeaconState, slot primitives.Slot) (cache.TrackedValidator, bool) {
	if features.Get().PrepareAllPayloads {
		return cache.TrackedValidator{Active: true}, true
	}
	id, err := helpers.BeaconProposerIndexAtSlot(s.ctx, st, slot)
	if err != nil {
		// It's acceptable to sometimes receive an error here
		assert.Sometimes(err != nil, "BeaconProposerIndexAtSlot returned error", map[string]interface{}{
			"error": err,
			"slot":  slot,
		})
		return cache.TrackedValidator{}, false
	}
	val, ok := s.cfg.TrackedValidatorsCache.Validator(id)
	if !ok {
		// It's acceptable that the validator is not found in the cache sometimes
		assert.Sometimes(!ok, "Validator not found in TrackedValidatorsCache", map[string]interface{}{
			"validator_index": id,
		})
		return cache.TrackedValidator{}, false
	}

	// Assert that the validator index matches in the cache
	assert.Always(val.ValidatorIndex == id, "Validator index matches in cache", map[string]interface{}{
		"validator_index":       id,
		"cache_validator_index": val.ValidatorIndex,
	})

	// Retrieve the validator from the state to verify its status
	stateValidator, err := st.Validator(id)
	if err != nil {
		// The validator should exist in the state if the index is valid
		assert.Unreachable("Validator should exist in state", map[string]interface{}{
			"validator_index": id,
			"error":           err,
		})
		return val, val.Active
	}

	// If the validator is marked as active, assert that it is active in the state
	if val.Active {
		currentEpoch := st.Epoch()
		activeInState := stateValidator.IsActive(currentEpoch)
		assert.Always(activeInState, "Validator is active in state", map[string]interface{}{
			"validator_index":  id,
			"current_epoch":    currentEpoch,
			"activation_epoch": stateValidator.ActivationEpoch,
			"exit_epoch":       stateValidator.ExitEpoch,
		})
	}

	return val, val.Active
}