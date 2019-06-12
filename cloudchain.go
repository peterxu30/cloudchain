// Package blockchain provides a simple blockchain implementation.
package cloudchain

import (
	"context"
	"errors"
	"fmt"
	"log"

	"google.golang.org/api/iterator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"cloud.google.com/go/firestore"
)

const (
	configCollection    = "config"
	cloudChainDoc       = "cloudChain"
	headDoc             = "head"
	difficultyDoc       = "difficulty"
	blocksCollection    = "blocks"
	initialized         = "initialized"
	blockHeader         = "header"
	batchSize           = 50
	addBlockChannelSize = 500
)

// Blockchain is the in-memory representation of the Blockchain.
// It does not actually store the blocks in-memory but rather the metadata required to read blocks from db.
type CloudChain struct {
	ProjectId       string
	Head            string
	Difficulty      int
	addBlockChannel chan addBlockPayload
}

type addBlockPayload struct {
	ctx               context.Context
	data              []byte
	addedBlockChannel chan *Block
	errorChannel      chan error
}

// NewCloudChain initializes a new CloudChain with the input proof of work difficulty and data for the genesis block.
// This function should only create a new CloudChain once for the given projectId.
func NewCloudChain(ctx context.Context, projectId string, difficulty int, genesisData []byte) (*CloudChain, error) {
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	configCollectionRef := client.Collection(configCollection)
	if configCollectionRef == nil {
		return nil, errors.New("Config collection not found")
	}

	var cloudChain CloudChain

	_, err = configCollectionRef.Doc(cloudChainDoc).Get(ctx)
	switch {
	case grpc.Code(err) == codes.NotFound:
		// initialize the cloudchain
		blocksCollectionRef := client.Collection(blocksCollection)
		genesisBlock := newGenesisBlock(genesisData)
		genesisBlockHash := genesisBlock.Header.Hash

		// store the genesis block
		_, err := blocksCollectionRef.Doc(genesisBlock.Header.Hash).Set(ctx, genesisBlock)
		if err != nil {
			return nil, err
		}

		cloudChain = CloudChain{
			ProjectId:  projectId,
			Head:       genesisBlockHash,
			Difficulty: difficulty,
		}
	case err == nil:
		// retrieve values from Firestore
		cloudChainSnap, err := configCollectionRef.Doc(cloudChainDoc).Get(ctx)
		if err != nil {
			return nil, err
		}

		err = cloudChainSnap.DataTo(&cloudChain)
		if err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	cloudChain.addBlockChannel = make(chan addBlockPayload, addBlockChannelSize)
	cloudChain.addBlocksAsync()
	return &cloudChain, nil
}

func newGenesisBlock(data []byte) *Block {
	if data == nil {
		data = []byte("Genesis Block")
	}
	return newBlock(
		0,
		"",
		data)
}

func (cc *CloudChain) addBlocksAsync() {
	go addBlocksRecoverer(5, addBlocksFromBlockChannel, cc)
}

//TODO: Batch this
func addBlocksFromBlockChannel(cc *CloudChain) {
	for {
		payload, ok := <-cc.addBlockChannel
		if !ok {
			// channel closed
			log.Println("addBlockChannel closed.")
			payload.errorChannel <- errors.New("addBlockChannel closed")
			close(payload.addedBlockChannel)
			close(payload.errorChannel)
			return
		}

		ctx := payload.ctx
		block := newBlock(cc.Difficulty, cc.Head, payload.data)
		blockHash := block.Header.Hash

		client, err := firestore.NewClient(ctx, cc.ProjectId)
		if err != nil {
			log.Panicln(err)
		}
		defer client.Close()

		configCollectionRef := client.Collection(configCollection)
		blocksCollectionRef := client.Collection(blocksCollection)

		// check if block already exists
		doc, err := blocksCollectionRef.Doc(blockHash).Get(ctx)

		switch {
		case doc != nil && doc.Exists():
			err = &collisionError{block.Header.Hash}
			fallthrough
		case err != nil && grpc.Code(err) != codes.NotFound: // NotFound err is acceptable and is satisfied in the first case
			log.Println(err)
			payload.errorChannel <- err
			close(payload.addedBlockChannel)
			close(payload.errorChannel)
			continue
		}

		// Add block to store
		_, err = blocksCollectionRef.Doc(blockHash).Set(ctx, block)
		if err != nil {
			log.Println(err)
			payload.errorChannel <- err
			close(payload.addedBlockChannel)
			close(payload.errorChannel)
			continue
		}

		// Set block as new head
		oldBlockHash := cc.Head
		cc.Head = blockHash

		_, err = configCollectionRef.Doc(cloudChainDoc).Set(ctx, cc)
		if err != nil {
			cc.Head = oldBlockHash
			cc.deleteBadBlock(ctx, blocksCollectionRef, blockHash) //make this a standalone method as opposed to attaching it to cloudchain?
			payload.errorChannel <- err
			close(payload.addedBlockChannel)
			close(payload.errorChannel)
			continue
		}

		payload.addedBlockChannel <- block
		close(payload.addedBlockChannel)
		close(payload.errorChannel)
	}
}

func addBlocksRecoverer(maxPanics int, f func(cc *CloudChain), cc *CloudChain) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
			if maxPanics == 0 {
				panic("AddBlocksAsync has exceeded the panic limit.")
			} else {
				go addBlocksRecoverer(maxPanics-1, f, cc)
			}
		}
	}()
	f(cc)
}

// AddBlock adds a block containing the input data to the front of the CloudChain.
// This is an asynchronous nonblocking call that returns two channels each of buffer size 1.
// The first channel is of type *Block and will yield the added block when the add operation is complete.
// The second channel is of type error and will yield any error that has occured in the add operation. If an error is yielded, the *Block channel will have never have values.
// When the asynchronous add is complete, both channels will be closed so it is valid to check for such.
func (cc *CloudChain) AddBlock(ctx context.Context, data []byte) (<-chan *Block, <-chan error) {
	payload := addBlockPayload{
		ctx:               ctx,
		data:              data,
		addedBlockChannel: make(chan *Block, 1),
		errorChannel:      make(chan error, 1),
	}
	cc.addBlockChannel <- payload

	return payload.addedBlockChannel, payload.errorChannel
}

func (cc *CloudChain) deleteBadBlock(ctx context.Context, blocksCollectionRef *firestore.CollectionRef, hashString string) {
	go func() {
		blocksCollectionRef.Doc(hashString).Delete(ctx)
	}()
}

func (cc *CloudChain) close() {
	close(cc.addBlockChannel)
}

// Iterator returns an iterator that traverses the Blockchain from most recent block to oldest.
func (cc *CloudChain) Iterator() (*CloudChainIterator, error) {
	return cc.IteratorAtBlock(cc.Head)
}

// IteratorAtBlock returns an iterator that traverses the Blockchain from the block with input hash to oldest.
func (cc *CloudChain) IteratorAtBlock(hash string) (*CloudChainIterator, error) {
	cci := &CloudChainIterator{
		projectId:   cc.ProjectId,
		currentHash: hash,
	}
	return cci, nil
}

// DeleteCloudChain deletes the input CloudChain and all stored data. This is permanent and irreversible.
func DeleteCloudChain(ctx context.Context, cc *CloudChain) error {
	cc.close()

	client, err := firestore.NewClient(ctx, cc.ProjectId)
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
