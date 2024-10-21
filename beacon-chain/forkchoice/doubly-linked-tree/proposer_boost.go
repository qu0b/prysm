package doublylinkedtree

import (
	"fmt"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/antithesishq/antithesis-sdk-go/assert"
)

// applyProposerBoostScore applies the current proposer boost scores to the
// relevant nodes.
func (f *ForkChoice) applyProposerBoostScore() error {
	s := f.store
	proposerScore := uint64(0)
	if s.previousProposerBoostRoot != params.BeaconConfig().ZeroHash {
		previousNode, ok := s.nodeByRoot[s.previousProposerBoostRoot]
		if !ok || previousNode == nil {
			log.WithError(errInvalidProposerBoostRoot).Errorf(fmt.Sprintf("invalid prev root %#x", s.previousProposerBoostRoot))
			assert.Unreachable("Invalid previous proposer boost root", map[string]any{
				"previousProposerBoostRoot": s.previousProposerBoostRoot,
			})
		} else {
			assert.AlwaysGreaterThanOrEqualTo(previousNode.balance, s.previousProposerBoostScore, "previousNode balance underflow", map[string]any{
				"previousNode.balance": previousNode.balance,
				"previousProposerBoostScore": s.previousProposerBoostScore,
				"previousNode": previousNode,
			})
			previousNode.balance -= s.previousProposerBoostScore
		}
	}

	if s.proposerBoostRoot != params.BeaconConfig().ZeroHash {
		currentNode, ok := s.nodeByRoot[s.proposerBoostRoot]
		if !ok || currentNode == nil {
			log.WithError(errInvalidProposerBoostRoot).Errorf(fmt.Sprintf("invalid current root %#x", s.proposerBoostRoot))
			assert.Unreachable("Invalid current proposer boost root", map[string]any{
				"proposerBoostRoot": s.proposerBoostRoot,
			})
		} else {
			proposerScore = (s.committeeWeight * params.BeaconConfig().ProposerScoreBoost) / 100
			assert.AlwaysLessThanOrEqualTo(proposerScore, s.committeeWeight, "proposerScore should not exceed committeeWeight", map[string]any{
				"proposerScore": proposerScore,
				"committeeWeight": s.committeeWeight,
				"ProposerScoreBoost": params.BeaconConfig().ProposerScoreBoost,
			})
			currentNode.balance += proposerScore
			assert.Always(currentNode.balance >= proposerScore, "currentNode balance overflow", map[string]any{
				"currentNode.balance": currentNode.balance,
				"proposerScore": proposerScore,
				"currentNode": currentNode,
			})
		}
	}
	s.previousProposerBoostRoot = s.proposerBoostRoot
	s.previousProposerBoostScore = proposerScore
	return nil
}

// ProposerBoost of fork choice store.
func (s *Store) proposerBoost() [fieldparams.RootLength]byte {
	return s.proposerBoostRoot
}
