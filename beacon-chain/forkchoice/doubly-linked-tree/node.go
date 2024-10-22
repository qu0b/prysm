package doublylinkedtree

import (
	"bytes"
	"context"

	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	forkchoice2 "github.com/prysmaticlabs/prysm/v5/consensus-types/forkchoice"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// ProcessAttestationsThreshold  is the number of seconds after which we
// process attestations for the current slot
const ProcessAttestationsThreshold = 10

// applyWeightChanges recomputes the weight of the node passed as an argument and all of its descendants,
// using the current balance stored in each node.
func (n *Node) applyWeightChanges(ctx context.Context) error {
	// Recursively calling the children to sum their weights.
	childrenWeight := uint64(0)
	for _, child := range n.children {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Add assertion to ensure child is not nil
		assert.Always(child != nil, "Child node should not be nil in applyWeightChanges", map[string]any{"parentRoot": n.root})
		if err := child.applyWeightChanges(ctx); err != nil {
			return err
		}
		childrenWeight += child.weight
	}
	if n.root == params.BeaconConfig().ZeroHash {
		return nil
	}
	n.weight = n.balance + childrenWeight
	// Add assertion to ensure weight is at least balance
	assert.Always(n.weight >= n.balance, "Node weight must be at least equal to balance", map[string]any{"nodeRoot": n.root, "balance": n.balance, "weight": n.weight})
	return nil
}

// updateBestDescendant updates the best descendant of this node and its
// children.
func (n *Node) updateBestDescendant(ctx context.Context, justifiedEpoch, finalizedEpoch, currentEpoch primitives.Epoch) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(n.children) == 0 {
		n.bestDescendant = nil
		return nil
	}

	var bestChild *Node
	bestWeight := uint64(0)
	hasViableDescendant := false
	for _, child := range n.children {
		if child == nil {
			return errors.Wrap(ErrNilNode, "could not update best descendant")
		}
		if err := child.updateBestDescendant(ctx, justifiedEpoch, finalizedEpoch, currentEpoch); err != nil {
			return err
		}
		childLeadsToViableHead := child.leadsToViableHead(justifiedEpoch, currentEpoch)
		if childLeadsToViableHead && !hasViableDescendant {
			// The child leads to a viable head, but the current
			// parent's best child doesn't.
			bestWeight = child.weight
			bestChild = child
			hasViableDescendant = true
		} else if childLeadsToViableHead {
			// If both are viable, compare their weights.
			if child.weight == bestWeight {
				// Tie-breaker of equal weights by root.
				if bytes.Compare(child.root[:], bestChild.root[:]) > 0 {
					bestChild = child
				}
			} else if child.weight > bestWeight {
				bestChild = child
				bestWeight = child.weight
			}
		}
	}
	if hasViableDescendant {
		if bestChild.bestDescendant == nil {
			n.bestDescendant = bestChild
		} else {
			n.bestDescendant = bestChild.bestDescendant
		}
	} else {
		n.bestDescendant = nil
	}
	// Add assertion to ensure bestDescendant is set correctly
	assert.Always((hasViableDescendant && n.bestDescendant != nil) || (!hasViableDescendant && n.bestDescendant == nil), "Best descendant should be set correctly", map[string]any{"nodeRoot": n.root, "hasViableDescendant": hasViableDescendant, "bestDescendant": n.bestDescendant})
	return nil
}

// viableForHead returns true if the node is viable to head.
// Any node with different finalized or justified epoch than
// the ones in fork choice store should not be viable to head.
func (n *Node) viableForHead(justifiedEpoch, currentEpoch primitives.Epoch) bool {
	if justifiedEpoch == 0 {
		return true
	}
	// We use n.justifiedEpoch as the voting source because:
	//   1. if this node is from current epoch, n.justifiedEpoch is the realized justification epoch.
	//   2. if this node is from a previous epoch, n.justifiedEpoch has already been updated to the unrealized justification epoch.
	return n.justifiedEpoch == justifiedEpoch || n.justifiedEpoch+2 >= currentEpoch
}

func (n *Node) leadsToViableHead(justifiedEpoch, currentEpoch primitives.Epoch) bool {
	if n.bestDescendant == nil {
		return n.viableForHead(justifiedEpoch, currentEpoch)
	}
	return n.bestDescendant.viableForHead(justifiedEpoch, currentEpoch)
}

// setNodeAndParentValidated sets the current node and all the ancestors as validated (i.e. non-optimistic).
func (n *Node) setNodeAndParentValidated(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !n.optimistic {
		return nil
	}
	n.optimistic = false

	if n.parent == nil {
		return nil
	}
	return n.parent.setNodeAndParentValidated(ctx)
}

// arrivedEarly returns whether this node was inserted before the first
// threshold to orphan a block.
// Note that genesisTime has seconds granularity, therefore we use a strict
// inequality < here. For example a block that arrives 3.9999 seconds into the
// slot will have secs = 3 below.
func (n *Node) arrivedEarly(genesisTime uint64) (bool, error) {
	secs, err := slots.SecondsSinceSlotStart(n.slot, genesisTime, n.timestamp)
	// Add assertion that error is nil and secs is non-negative
	assert.Always(err == nil, "Error calculating seconds since slot start in arrivedEarly", map[string]any{"nodeRoot": n.root, "slot": n.slot, "timestamp": n.timestamp, "error": err})
	votingWindow := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	return secs < votingWindow, err
}

// arrivedAfterOrphanCheck returns whether this block was inserted after the
// intermediate checkpoint to check for candidate of being orphaned.
// Note that genesisTime has seconds granularity, therefore we use an
// inequality >= here. For example a block that arrives 10.00001 seconds into the
// slot will have secs = 10 below.
func (n *Node) arrivedAfterOrphanCheck(genesisTime uint64) (bool, error) {
	secs, err := slots.SecondsSinceSlotStart(n.slot, genesisTime, n.timestamp)
	// Add assertion that error is nil and secs is non-negative
	assert.Always(err == nil, "Error calculating seconds since slot start in arrivedAfterOrphanCheck", map[string]any{"nodeRoot": n.root, "slot": n.slot, "timestamp": n.timestamp, "error": err})
	return secs >= ProcessAttestationsThreshold, err
}

// nodeTreeDump appends to the given list all the nodes descending from this one
func (n *Node) nodeTreeDump(ctx context.Context, nodes []*forkchoice2.Node) ([]*forkchoice2.Node, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var parentRoot [32]byte
	if n.parent != nil {
		parentRoot = n.parent.root
	}
	thisNode := &forkchoice2.Node{
		Slot:                     n.slot,
		BlockRoot:                n.root[:],
		ParentRoot:               parentRoot[:],
		JustifiedEpoch:           n.justifiedEpoch,
		FinalizedEpoch:           n.finalizedEpoch,
		UnrealizedJustifiedEpoch: n.unrealizedJustifiedEpoch,
		UnrealizedFinalizedEpoch: n.unrealizedFinalizedEpoch,
		Balance:                  n.balance,
		Weight:                   n.weight,
		ExecutionOptimistic:      n.optimistic,
		ExecutionBlockHash:       n.payloadHash[:],
		Timestamp:                n.timestamp,
	}
	if n.optimistic {
		thisNode.Validity = forkchoice2.Optimistic
	} else {
		thisNode.Validity = forkchoice2.Valid
	}

	nodes = append(nodes, thisNode)
	var err error
	for _, child := range n.children {
		// Add assertion to ensure child is not nil
		assert.Always(child != nil, "Child node should not be nil in nodeTreeDump", map[string]any{"parentRoot": n.root})
		nodes, err = child.nodeTreeDump(ctx, nodes)
		if err != nil {
			return nil, err
		}
	}
	return nodes, nil
}
