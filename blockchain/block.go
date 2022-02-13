package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"log"
	"strconv"
)

// (very) minimal block
type Block struct {
	Hash     []byte
	Txs      []*Tx
	PrevHash []byte
	Nonce    int
}

func (block *Block) HashTxs() []byte {
	var txHashes [][]byte
	var txHash [32]byte

	// extract transactions
	for _, tx := range block.Txs {
		txHashes = append(txHashes, tx.ID)
	}
	txHash = sha256.Sum256(bytes.Join(txHashes, []byte{})) // hash transactions concatenated as bytes

	return txHash[:]
}

// mint block using proof of work
func CreateBlock(txs []*Tx, prevHash []byte) *Block {
	block := &Block{[]byte{}, txs, prevHash, 0}
	pow := NewProof(block)
	nonce, hash := pow.Run()

	block.Hash = hash[:]
	block.Nonce = nonce

	return block
}

// first block in a chain
func Genesis(coinbase *Tx) *Block {
	return CreateBlock([]*Tx{coinbase}, []byte{})
}

// dump single block
func (block *Block) PrintBlock() {
	fmt.Printf("Nonce      : %#x\n", block.Nonce)
	fmt.Printf("Hash       : %x\n", block.Hash)
	fmt.Printf("PrevHash   : %x\n", block.PrevHash)

	fmt.Println("Transactions")
	for _, tx := range block.Txs {
		tx.PrintTx()
	}

	pow := NewProof(block)
	fmt.Printf("ValidBlock : %s\n", strconv.FormatBool(pow.Validate()))
}

func (b *Block) Block2Bytes() []byte {
	var ret bytes.Buffer
	encoder := gob.NewEncoder(&ret)

	// encode block to bytes
	Handle(encoder.Encode(b))
	return ret.Bytes()
}

func Bytes2Block(data []byte) *Block {
	var block Block

	decoder := gob.NewDecoder(bytes.NewReader(data))

	// squash raw bytes back into block structure
	Handle(decoder.Decode(&block))
	return &block
}

// Panic on error
func Handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}
