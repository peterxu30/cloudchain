package cloudchain

import (
	"bytes"
	"encoding/gob"
	"strconv"
	"strings"
	"time"
)

// Block is the representation of a block on the CloudChain.
// It is made of a BlockHeader of metadata and a Data payload in the form of a byte array.
type Block struct {
	Header *BlockHeader
	Data   []byte
}

// BlockHeader stores metadata relevant to a Block.
type BlockHeader struct {
	Timestamp    int64
	Hash         string
	PreviousHash string
	Nonce        int64
	Difficulty   int
}

func newBlock(difficulty int, previousHash string, data []byte) *Block {
	header := &BlockHeader{
		Timestamp:    time.Now().Unix(),
		PreviousHash: previousHash,
		Difficulty:   difficulty,
	}

	block := &Block{
		Header: header,
		Data:   data,
	}

	pow := NewProofOfWork(block, difficulty)
	nonce, hash := pow.Run()

	header.Hash = convertHashToString(hash[:8])
	header.Nonce = nonce

	return block
}

func serializeBlock(block *Block) ([]byte, error) {
	var result bytes.Buffer
	encoder := gob.NewEncoder(&result)

	err := encoder.Encode(block)

	if err != nil {
		return nil, err
	}

	return result.Bytes(), nil
}

func deserializeBlock(d []byte) (*Block, error) {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(d))
	err := decoder.Decode(&block)

	if err != nil {
		return nil, err
	}

	return &block, nil
}

func (header *BlockHeader) IsLastBlock() bool {
	return len(header.PreviousHash) == 0
}

func convertHashToString(hash []byte) string {
	var sb strings.Builder
	for _, e := range hash {
		n := strconv.Itoa(int(e))
		sb.WriteString(n)
	}
	return sb.String()
}
