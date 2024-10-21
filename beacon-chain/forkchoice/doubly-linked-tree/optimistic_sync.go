package doublylinkedtree

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"

	"github.com/antithesishq/antithesis-sdk-go/assert"
)

func (s *Store) setOptimisticToInvalid(ctx context.Context, root, parentRoot, lastValidHash [32]byte) ([][32]byte, error) {
	invalidRoots := make([][32]byte, 0)
	node, ok := s.nodeByRoot[root]
	if !ok {
		node, ok = s.nodeByRoot[parentRoot]
		if !ok || node == nil {
			return invalidRoots, errors.Wrap(ErrNilNode, "could not set node to invalid")
		}
		// return early if the parent is LVH
		if node.payloadHash == lastValidHash {
			return invalidRoots, nil
		}
	} else {
		if node == nil {
			return invalidRoots, errors.Wrap(ErrNilNode, "could not set node to invalid")
		}
		// Assert that node.parent is not nil
		assert.Always(node.parent != nil, "node.parent is not nil in setOptimisticToInvalid else block", map[string]any{
			"node_root": node.root,
		})
		if node.parent.root != parentRoot {
			return invalidRoots, errInvalidParentRoot
		}
	}
	firstInvalid := node
	for ; firstInvalid.parent != nil && firstInvalid.parent.payloadHash != lastValidHash; firstInvalid = firstInvalid.parent {
		if ctx.Err() != nil {
			return invalidRoots, ctx.Err()
		}
	}
	// Deal with the case that the last valid payload is in a different fork
	// This means we are dealing with an EE that does not follow the spec
	if firstInvalid.parent == nil {
		// return early if the invalid node was not imported
		if node.root == parentRoot {
			return invalidRoots, nil
		}
		firstInvalid = node
	}
	// Assert that firstInvalid is not nil before calling removeNode
	assert.Always(firstInvalid != nil, "firstInvalid is not nil before removeNode", map[string]any{
		"firstInvalid_root": firstInvalid.root,
	})
	return s.removeNode(ctx, firstInvalid)
}

// removeNode removes the node with the given root and all of its children
// from the Fork Choice Store.
func (s *Store) removeNode(ctx context.Context, node *Node) ([][32]byte, error) {
	invalidRoots := make([][32]byte, 0)

	if node == nil {
		return invalidRoots, errors.Wrap(ErrNilNode, "could not remove node")
	}
	if !node.optimistic || node.parent == nil {
		return invalidRoots, errInvalidOptimisticStatus
	}

	children := node.parent.children
	if len(children) == 1 {
		node.parent.children = []*Node{}
	} else {
		foundNode := false
		for i, n := range children {
			if n == node {
				foundNode = true
				if i != len(children)-1 {
					children[i] = children[len(children)-1]
				}
				node.parent.children = children[:len(children)-1]
				break
			}
		}
		// Assert that we found the node in parent's children
		assert.Always(foundNode, "node found in parent.children during removal", map[string]any{
			"node_root":   node.root,
			"parent_root": node.parent.root,
		})
	}
	return s.removeNodeAndChildren(ctx, node, invalidRoots)
}

// removeNodeAndChildren removes `node` and all of its descendant from the Store
func (s *Store) removeNodeAndChildren(ctx context.Context, node *Node, invalidRoots [][32]byte) ([][32]byte, error) {
	var err error
	for _, child := range node.children {
		if ctx.Err() != nil {
			return invalidRoots, ctx.Err()
		}
		if invalidRoots, err = s.removeNodeAndChildren(ctx, child, invalidRoots); err != nil {
			return invalidRoots, err
		}
	}
	invalidRoots = append(invalidRoots, node.root)
	if node.root == s.proposerBoostRoot {
		s.proposerBoostRoot = [32]byte{}
	}
	if node.root == s.previousProposerBoostRoot {
		s.previousProposerBoostRoot = params.BeaconConfig().ZeroHash
		s.previousProposerBoostScore = 0
	}
	// Assert that node exists in s.nodeByRoot before deleting
	assert.Always(s.nodeByRoot[node.root] != nil, "node exists in s.nodeByRoot before deleting", map[string]any{
		"node_root": node.root,
	})
	delete(s.nodeByRoot, node.root)
	delete(s.nodeByPayload, node.payloadHash)
	// Assert that node no longer exists in s.nodeByRoot after deleting
	assert.Always(s.nodeByRoot[node.root] == nil, "node no longer exists in s.nodeByRoot after deleting", map[string]any{
		"node_root": node.root,
	})
	return invalidRoots, nil
}
