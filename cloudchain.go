// Package blockchain provides a simple blockchain implementation.
package cloudchain

import (
	"context"

	"google.golang.org/api/iterator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"cloud.google.com/go/firestore"
)

const (
	configCollection = "config"
	headDoc          = "head"
	difficultyDoc    = "difficulty"
	blocksCollection = "blocks"
	initialized      = "initialized"
	batchSize        = 50
)

// Blockchain is the in-memory representation of the Blockchain.
// It does not actually store the blocks in-memory but rather the metadata required to read blocks from db.
type CloudChain struct {
	projectId  string
	head       []byte
	difficulty int
}

// BlockchainIterator is the iterator that traverses the Blockchain.
type CloudChainIterator struct {
	currentHash []byte
	ref         *firestore.CollectionRef
	ctx         context.Context
}

// NewCloudChain initializes a new CloudChain with the input proof of work difficulty and data for the genesis block.
// This function should only create a new CloudChain once.
func NewCloudChain(ctx context.Context, projectId string, difficulty int, genesisData []byte) (*CloudChain, error) {
	// client used to initialize data. DO NOT store client in struct. RECREATE CLIENT ON EVERY CLOUDCHAIN OP
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	configCollectionRef := client.Collection(configCollection)
	dsnap, err := configCollectionRef.Doc(initialized).Get(ctx)
	if grpc.Code(err) == codes.NotFound || !dsnap.Exists() {
		// initialize the cloudchain
		_, err = configCollectionRef.Doc(difficultyDoc).Set(ctx, difficulty)
		if err != nil {
			return nil, err
		}

		blocksCollectionRef := client.Collection(blocksCollection)
		genesisBlock := newGenesisBlock(genesisData)

		genesisBlockHash := genesisBlock.Header.Hash

		_, err := blocksCollectionRef.Doc(string(genesisBlockHash)).Set(ctx, genesisBlock)
		if err != nil {
			return nil, err
		}

		_, err = configCollectionRef.Doc(headDoc).Set(ctx, genesisBlockHash)
		if err != nil {
			return nil, err
		}

		_, err = configCollectionRef.Doc(initialized).Set(ctx, true)
		if err != nil {
			return nil, err
		}

		return &CloudChain{
			projectId:  projectId,
			head:       genesisBlockHash,
			difficulty: difficulty,
		}, nil
	} else if err == nil {
		// retrieve values from Firestore
		headSnap, err := configCollectionRef.Doc(headDoc).Get(ctx)
		if err != nil {
			return nil, err
		}

		var headVal []byte
		err = headSnap.DataTo(&headVal)
		if err != nil {
			return nil, err
		}

		difficultySnap, err := configCollectionRef.Doc(difficultyDoc).Get(ctx)
		if err != nil {
			return nil, err
		}

		var difficultyVal int
		err = difficultySnap.DataTo(&difficultyVal)
		if err != nil {
			return nil, err
		}

		return &CloudChain{
			projectId:  projectId,
			head:       headVal,
			difficulty: difficultyVal,
		}, nil
	} else {
		return nil, err
	}
}

func newGenesisBlock(data []byte) *Block {
	if data == nil {
		data = []byte("Genesis Block")
	}
	return newBlock(
		0,
		nil,
		data)
}

// AddBlock adds a block containing the input data to the front of the CloudChain.
func (cc *CloudChain) AddBlock(ctx context.Context, data []byte) (*Block, error) {
	client, err := firestore.NewClient(ctx, cc.projectId)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	configCollectionRef := client.Collection(configCollection)
	blocksCollectionRef := client.Collection(blocksCollection)

	newBlock := newBlock(cc.difficulty, cc.head, data)
	newBlockHashString := string(newBlock.Header.Hash)

	//check if block already exists
	_, err = blocksCollectionRef.Doc(newBlockHashString).Get(ctx)
	if grpc.Code(err) != codes.NotFound {
		return nil, &collisionError{newBlock.Header.Hash}
	}

	_, err = blocksCollectionRef.Doc(newBlockHashString).Set(ctx, newBlock)
	if err != nil {
		return nil, err
	}

	_, err = configCollectionRef.Doc(headDoc).Set(ctx, newBlock.Header.Hash)
	if err != nil {
		cc.deleteBadBlock(ctx, blocksCollectionRef, newBlockHashString)
		return nil, err
	}

	return newBlock, nil
}

func (cc *CloudChain) deleteBadBlock(ctx context.Context, blocksCollectionRef *firestore.CollectionRef, hashString string) {
	go func() {
		blocksCollectionRef.Doc(hashString).Delete(ctx)
	}()
}

// Difficulty returns the difficulty of the Blockchain.
func (cc *CloudChain) Difficulty() int {
	return cc.difficulty
}

// Iterator returns an iterator that traverses the Blockchain from most recent block to oldest.
func (cc *CloudChain) Iterator(ctx context.Context) (*CloudChainIterator, error) {
	return cc.IteratorAtBlock(ctx, cc.head)
}

// IteratorAtBlock returns an iterator that traverses the Blockchain from the block with input hash to oldest.
func (cc *CloudChain) IteratorAtBlock(ctx context.Context, hash []byte) (*CloudChainIterator, error) {
	client, err := firestore.NewClient(ctx, cc.projectId)
	if err != nil {
		return nil, err
	}

	ref := client.Collection(blocksCollection)
	cci := &CloudChainIterator{
		currentHash: hash,
		ref:         ref,
	}
	return cci, nil
}

// Next returns the next block in the Blockchain.
func (cci *CloudChainIterator) Next() (*Block, error) {
	if cci.currentHash == nil {
		return nil, nil
	}

	blockSnap, err := cci.ref.Doc(string(cci.currentHash)).Get(cci.ctx)
	if err != nil {
		return nil, err
	}

	var block *Block
	err = blockSnap.DataTo(block)
	if err != nil {
		return nil, err
	}

	cci.currentHash = block.Header.PreviousHash
	return block, nil
}

// DeleteBlockchain deletes the input Blockchain and all stored data. This is permanent and irreversible.
func DeleteCloudChain(ctx context.Context, cc *CloudChain) error {
	client, err := firestore.NewClient(ctx, cc.projectId)
	if err != nil {
		return err
	}
	defer client.Close()

	batchDeleteDocs(ctx, client, configCollection, batchSize)
	if err != nil {
		return err
	}

	batchDeleteDocs(ctx, client, blocksCollection, batchSize)
	if err != nil {
		return err
	}

	return nil
}

func batchDeleteDocs(ctx context.Context, client *firestore.Client, collectionName string, batchSize int) error {
	iter := client.Collection(collectionName).Documents(ctx)
	batch := client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}

		batch.Delete(doc.Ref)
	}

	_, err := batch.Commit(ctx)
	if err != nil {
		return err
	}
	return nil
}
