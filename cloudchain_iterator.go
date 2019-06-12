package cloudchain

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
)

// BlockchainIterator is the iterator that traverses the Blockchain.
type CloudChainIterator struct {
	projectId   string
	currentHash string
}

// Next returns the next block in the Blockchain.
func (cci *CloudChainIterator) Next(ctx context.Context) (*Block, error) {
	if cci.currentHash == "" {
		return nil, &StopIterationError{}
	}

	client, err := firestore.NewClient(ctx, cci.projectId)
	if err != nil {
		return nil, err
	}

	ref := client.Collection(blocksCollection)

	blockSnap, err := ref.Doc(cci.currentHash).Get(ctx)
	if err != nil {
		errString := fmt.Sprintf("Failed to retrieve block with hash %s. Inner error: %s", cci.currentHash, err.Error())
		return nil, errors.New(errString)
	}

	var block Block
	err = blockSnap.DataTo(&block)
	if err != nil {
		errString := fmt.Sprintf("Block with hash %s does not exist. Inner error: %s", cci.currentHash, err.Error())
		return nil, errors.New(errString)
	}

	cci.currentHash = block.Header.PreviousHash
	return &block, nil
}
