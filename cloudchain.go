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

// BlockchainIterator is the iterator that traverses the Blockchain.
type CloudChainIterator struct {
	projectId   string
	currentHash string
}

// NewCloudChain initializes a new CloudChain with the input proof of work difficulty and data for the genesis block.
// This function should only create a new CloudChain once for the given projectId.
func NewCloudChain(ctx context.Context, projectId string, difficulty int, genesisData []byte) (*CloudChain, error) {
	// client used to initialize data. DO NOT store client in struct. RECREATE CLIENT ON EVERY CLOUDCHAIN OP
	client, err := firestore.NewClient(ctx, projectId)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	configCollectionRef := client.Collection(configCollection)
	if configCollectionRef == nil {
		return nil, errors.New("Config collection not found")
	}

	_, err = configCollectionRef.Doc(cloudChainDoc).Get(ctx)
	if grpc.Code(err) == codes.NotFound {
		// initialize the cloudchain
		blocksCollectionRef := client.Collection(blocksCollection)
		genesisBlock := newGenesisBlock(genesisData)
		genesisBlockHash := genesisBlock.Header.Hash

		// store the genesis block
		_, err := blocksCollectionRef.Doc(genesisBlock.Header.Hash).Set(ctx, genesisBlock)
		if err != nil {
			return nil, err
		}

		cloudChain := &CloudChain{
			ProjectId:       projectId,
			Head:            genesisBlockHash,
			Difficulty:      difficulty,
			addBlockChannel: make(chan addBlockPayload, addBlockChannelSize),
		}

		_, err = configCollectionRef.Doc(cloudChainDoc).Set(ctx, cloudChain)
		if err != nil {
			return nil, err
		}

		cloudChain.addBlocksAsync(ctx)
		return cloudChain, nil
	} else if err == nil {
		// retrieve values from Firestore
		cloudChainSnap, err := configCollectionRef.Doc(cloudChainDoc).Get(ctx)
		if err != nil {
			return nil, err
		}

		var cloudChain CloudChain
		err = cloudChainSnap.DataTo(&cloudChain)
		if err != nil {
			return nil, err
		}

		cloudChain.addBlockChannel = make(chan addBlockPayload, addBlockChannelSize)

		cloudChain.addBlocksAsync(ctx)
		return &cloudChain, nil
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
		"",
		data)
}

func (cc *CloudChain) addBlocksAsync(ctx context.Context) {
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
		_, err = blocksCollectionRef.Doc(blockHash).Get(ctx)
		if grpc.Code(err) != codes.NotFound {
			// err := &collisionError{block.Header.Hash}
			log.Println(err)
			payload.errorChannel <- err
			close(payload.addedBlockChannel)
			close(payload.errorChannel)
			continue
		}

		_, err = blocksCollectionRef.Doc(blockHash).Set(ctx, block)
		if err != nil {
			log.Println(err)
			payload.errorChannel <- err
			close(payload.addedBlockChannel)
			close(payload.errorChannel)
			continue
		}

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

// Consider making the channel pass a tuple of (block, wg/lock etc) s.t. when the AddBlockAsync goroutine adds this block, it calls Done/unlocks the wg/lock to tell this call it's completed.
// This method will become a blocking method.
// Currently not blocking
func (cc *CloudChain) AddBlockExperimental(ctx context.Context, data []byte) (<-chan *Block, <-chan error) {
	payload := addBlockPayload{
		ctx:               ctx,
		data:              data,
		addedBlockChannel: make(chan *Block, 1),
		errorChannel:      make(chan error, 1),
	}
	cc.addBlockChannel <- payload

	return payload.addedBlockChannel, payload.errorChannel
}

// AddBlock adds a block containing the input data to the front of the CloudChain.
func (cc *CloudChain) AddBlock(ctx context.Context, data []byte) (*Block, error) {
	client, err := firestore.NewClient(ctx, cc.ProjectId)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	configCollectionRef := client.Collection(configCollection)
	blocksCollectionRef := client.Collection(blocksCollection)

	newBlock := newBlock(cc.Difficulty, cc.Head, data)
	newBlockHash := newBlock.Header.Hash

	// check if block already exists
	_, err = blocksCollectionRef.Doc(newBlockHash).Get(ctx)
	if grpc.Code(err) != codes.NotFound {
		return nil, &collisionError{newBlock.Header.Hash}
	}

	_, err = blocksCollectionRef.Doc(newBlockHash).Set(ctx, newBlock)
	if err != nil {
		return nil, err
	}

	oldBlockHash := cc.Head
	cc.Head = newBlockHash

	_, err = configCollectionRef.Doc(cloudChainDoc).Set(ctx, cc)
	if err != nil {
		cc.Head = oldBlockHash
		cc.deleteBadBlock(ctx, blocksCollectionRef, newBlockHash)
		return nil, err
	}

	return newBlock, nil
}

func (cc *CloudChain) deleteBadBlock(ctx context.Context, blocksCollectionRef *firestore.CollectionRef, hashString string) {
	go func() {
		blocksCollectionRef.Doc(hashString).Delete(ctx)
	}()
}

func (cc *CloudChain) Close() {
	close(cc.addBlockChannel)
}

// Iterator returns an iterator that traverses the Blockchain from most recent block to oldest.
// TODO: Change to pass in ctx every next call.
func (cc *CloudChain) Iterator() (*CloudChainIterator, error) {
	return cc.IteratorAtBlock(cc.Head)
}

// IteratorAtBlock returns an iterator that traverses the Blockchain from the block with input hash to oldest.
func (cc *CloudChain) IteratorAtBlock(hash string) (*CloudChainIterator, error) {
	// client, err := firestore.NewClient(ctx, cci.ProjectId)
	// if err != nil {
	// 	return nil, err
	// }

	// ref := client.Collection(blocksCollection)
	cci := &CloudChainIterator{
		projectId:   cc.ProjectId,
		currentHash: hash,
		// ref:         ref,
		// ctx:         ctx,
	}
	return cci, nil
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

// DeleteCloudChain deletes the input CloudChain and all stored data. This is permanent and irreversible.
func DeleteCloudChain(ctx context.Context, cc *CloudChain) error {
	cc.Close()

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
