package cloudchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"log"
	"math"
	"math/big"
)

// ProofOfWork contains the information necessary to run the proof of work scheme used when adding a block to the CloudChain.
type ProofOfWork struct {
	block      *Block
	target     *big.Int
	targetBits int
}

func NewProofOfWork(block *Block, targetBits int) *ProofOfWork {
	target := big.NewInt(1)
	target.Lsh(target, uint(256-targetBits))

	return &ProofOfWork{
		block:      block,
		target:     target,
		targetBits: targetBits,
	}
}

func (pow *ProofOfWork) prepareData(nonce int64) []byte {
	data := bytes.Join(
		[][]byte{
			[]byte(pow.block.Header.PreviousHash),
			pow.block.Data,
			IntToBytes(pow.block.Header.Timestamp),
			IntToBytes(int64(pow.targetBits)),
			IntToBytes(nonce),
		},
		[]byte{},
	)

	return data
}

// Run computes the nonce that satisfies the proof of work.
func (pow *ProofOfWork) Run() (int64, []byte) {
	var hashInt big.Int
	var hash [32]byte
	var nonce int64

	for nonce < math.MaxInt64 {
		data := pow.prepareData(nonce)
		hash = sha256.Sum256(data)
		hashInt.SetBytes(hash[:])

		if hashInt.Cmp(pow.target) == -1 {
			break
		} else {
			nonce++
		}
	}

	return nonce, hash[:]
}

// Validate will check if the proof of work is valid.
func (pow *ProofOfWork) Validate() bool {
	var hashInt big.Int

	data := pow.prepareData(pow.block.Header.Nonce)
	hash := sha256.Sum256(data)
	hashInt.SetBytes(hash[:])

	isValid := hashInt.Cmp(pow.target) == -1
	return isValid
}

func IntToBytes(num int64) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}
