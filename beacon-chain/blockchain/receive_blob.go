package blockchain

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/antithesishq/antithesis-sdk-go/assert"
)

// SendNewBlobEvent sends a message to the BlobNotifier channel that the blob
// for the block root `root` is ready in the database
func (s *Service) sendNewBlobEvent(root [32]byte, index uint64) {
	assert.Always(s.blobNotifiers != nil, "blobNotifiers should not be nil", map[string]any{"Index": index})
	assert.Always(root != [32]byte{}, "Root should not be zero", map[string]any{"Index": index})

	s.blobNotifiers.notifyIndex(root, index)
}

// ReceiveBlob saves the blob to database and sends the new event
func (s *Service) ReceiveBlob(ctx context.Context, b blocks.VerifiedROBlob) error {
	assert.Always(b.BlockRoot() != [32]byte{}, "Blob BlockRoot should not be zero", map[string]any{"Index": b.Index})
	assert.Always(s.blobStorage != nil, "blobStorage should not be nil", nil)

	if err := s.blobStorage.Save(b); err != nil {
		return err
	}

	s.sendNewBlobEvent(b.BlockRoot(), b.Index)
	return nil
}