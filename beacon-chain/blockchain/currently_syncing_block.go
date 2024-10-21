package blockchain

import (
	"sync"
	"github.com/antithesishq/antithesis-sdk-go/assert"
)

type currentlySyncingBlock struct {
	sync.Mutex
	roots map[[32]byte]struct{}
}

func (b *currentlySyncingBlock) set(root [32]byte) {
	b.Lock()
	defer b.Unlock()
	assert.Always(b.roots != nil, "Map b.roots should not be nil in set", nil)
	if _, exists := b.roots[root]; exists {
		assert.Unreachable("Attempting to set a root that is already syncing", map[string]any{"root": root})
	}
	b.roots[root] = struct{}{}
}

func (b *currentlySyncingBlock) unset(root [32]byte) {
	b.Lock()
	defer b.Unlock()
	assert.Always(b.roots != nil, "Map b.roots should not be nil in unset", nil)
	if _, exists := b.roots[root]; !exists {
		assert.Unreachable("Attempting to unset a root that is not currently syncing", map[string]any{"root": root})
	}
	delete(b.roots, root)
}

func (b *currentlySyncingBlock) isSyncing(root [32]byte) bool {
	b.Lock()
	defer b.Unlock()
	assert.Always(b.roots != nil, "Map b.roots should not be nil in isSyncing", nil)
	_, ok := b.roots[root]
	return ok
}
