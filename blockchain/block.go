package blockchain

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"strconv"
	"time"
)

// (very) minimal block
type Block struct {
	Timestamp int64
	Hash      []byte
	Txs       []*Tx
	PrevHash  []byte
	Nonce     int
	Height    int
}

func (block *Block) HashTxs() []byte {
	var txHashes [][]byte

	// extract transactions
	for _, tx := range block.Txs {
		txHashes = append(txHashes, tx.ToBytes())
	}

	// create merkle tree
	tree := NewMerkleTree(txHashes)

	return tree.RootNode.Data
}

// mint block using proof of work
func CreateBlock(txs []*Tx, prevHash []byte, height int) *Block {
	block := &Block{
		Timestamp: time.Now().Unix(),
		Hash:      []byte{},
		Txs:       txs,
		PrevHash:  prevHash,
		Nonce:     0,
		Height:    height,
	}
	pow := NewProof(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// first block in a chain
func Genesis(coinbase *Tx) *Block {
	return CreateBlock([]*Tx{coinbase}, []byte{}, 0)
}

// dump single block
func (block *Block) PrintBlock() {
	fmt.Printf("Timestamp : %v\n", time.Unix(block.Timestamp, 0))
	fmt.Printf("Nonce     : %#x\n", block.Nonce)
	fmt.Printf("Hash      : %x\n", block.Hash)
	fmt.Printf("PrevHash  : %x\n", block.PrevHash)
	fmt.Printf("Height    : %d\n", block.Height)

	fmt.Println("Transactions")
	for _, tx := range block.Txs {
		tx.PrintTx()
	}

	pow := NewProof(block)
	fmt.Printf("ValidBlock : %s\n", strconv.FormatBool(pow.Validate()))
}

func (b *Block) ToBytes() []byte {
	var ret bytes.Buffer
	encoder := gob.NewEncoder(&ret)

	// encode block to bytes
	HandleErr(encoder.Encode(b))
	return ret.Bytes()
}

func Bytes2Block(data []byte) *Block {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(data))

	// squash raw bytes back into block structure
	HandleErr(decoder.Decode(&block))
	return &block
}

// Panic on error
func HandleErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}
