package blockchain

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"

	"github.com/antithesishq/antithesis-sdk-go/assert"
)

// This saves a beacon block to the initial sync blocks cache. It rate limits how many blocks
// the cache keeps in memory (2 epochs worth of blocks) and saves them to DB when it hits this limit.
func (s *Service) saveInitSyncBlock(ctx context.Context, r [32]byte, b interfaces.ReadOnlySignedBeaconBlock) error {
	// Assert that block 'b' is not nil
	assert.Always(b != nil, "saveInitSyncBlock: block is not nil", map[string]any{"block_root": r})

	s.initSyncBlocksLock.Lock()
	s.initSyncBlocks[r] = b
	numBlocks := len(s.initSyncBlocks)
	s.initSyncBlocksLock.Unlock()
	if uint64(numBlocks) > initialSyncBlockCacheSize {
		blocksToSave := s.getInitSyncBlocks()
		// Assert that blocksToSave is not empty
		assert.Always(len(blocksToSave) > 0, "saveInitSyncBlock: blocks to save is not empty", map[string]any{"num_blocks": len(blocksToSave)})

		if err := s.cfg.BeaconDB.SaveBlocks(ctx, blocksToSave); err != nil {
			return err
		}
		s.clearInitSyncBlocks()
	}
	return nil
}

// This checks if a beacon block exists in the initial sync blocks cache using the root
// of the block.
func (s *Service) hasInitSyncBlock(r [32]byte) bool {
	s.initSyncBlocksLock.RLock()
	defer s.initSyncBlocksLock.RUnlock()
	_, ok := s.initSyncBlocks[r]
	return ok
}

// Returns true if a block for root `r` exists in the initial sync blocks cache or the DB.
func (s *Service) hasBlockInInitSyncOrDB(ctx context.Context, r [32]byte) bool {
	if s.hasInitSyncBlock(r) {
		return true
	}
	return s.cfg.BeaconDB.HasBlock(ctx, r)
}

// Returns block for a given root `r` from either the initial sync blocks cache or the DB.
// Error is returned if the block is not found in either cache or DB.
func (s *Service) getBlock(ctx context.Context, r [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	s.initSyncBlocksLock.RLock()

	// Check cache first because it's faster.
	b, ok := s.initSyncBlocks[r]
	s.initSyncBlocksLock.RUnlock()
	if ok {
		// Assert that 'b' is not nil when found in cache
		assert.Always(b != nil, "getBlock: block from cache is not nil", map[string]any{"block_root": r})
	} else {
		var err error
		b, err = s.cfg.BeaconDB.Block(ctx, r)
		if err != nil {
			return nil, errors.Wrap(err, "could not retrieve block from db")
		}
		// Assert that 'b' is not nil when retrieved from DB
		assert.Always(b != nil, "getBlock: block from DB is not nil", map[string]any{"block_root": r})
	}
	if err := blocks.BeaconBlockIsNil(b); err != nil {
		return nil, errBlockNotFoundInCacheOrDB
	}
	return b, nil
}

// This retrieves all the beacon blocks from the initial sync blocks cache, the returned
// blocks are unordered.
func (s *Service) getInitSyncBlocks() []interfaces.ReadOnlySignedBeaconBlock {
	s.initSyncBlocksLock.RLock()
	defer s.initSyncBlocksLock.RUnlock()

	blks := make([]interfaces.ReadOnlySignedBeaconBlock, 0, len(s.initSyncBlocks))
	for _, b := range s.initSyncBlocks {
		blks = append(blks, b)
	}
	// Assert that number of blocks matches the length of s.initSyncBlocks
	assert.Always(len(blks) == len(s.initSyncBlocks), "getInitSyncBlocks: extracted blocks count matches", map[string]any{"num_blocks": len(blks)})

	return blks
}

// This clears out the initial sync blocks cache.
func (s *Service) clearInitSyncBlocks() {
	s.initSyncBlocksLock.Lock()
	defer s.initSyncBlocksLock.Unlock()
	s.initSyncBlocks = make(map[[32]byte]interfaces.ReadOnlySignedBeaconBlock)
	// Assert that initSyncBlocks is now empty
	assert.Always(len(s.initSyncBlocks) == 0, "clearInitSyncBlocks: initSyncBlocks is empty after clearing", nil)
}
