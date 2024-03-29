package blockchain

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"strconv"
	"time"
)

type Block struct {
	Hash       []byte
	Nonce      int
	Timestamp  int64
	Difficulty int64
	PrevHash   []byte
	Height     int64
	Txs        []*Tx
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

func GetDifficulty(prevHash []byte, height int64, chain *BlockChain) int64 {

	// if no previous block,
	if prevHash == nil {
		return InitialDifficulty
	}
	prevBlock, err := chain.GetBlockByHash(prevHash)
	HandleErr(err)

	timeDiff := time.Now().Unix() - prevBlock.Timestamp
	if height%AdjustmentInverval == 0 {
		if timeDiff > BlockMiningInterval*2 {
			return prevBlock.Difficulty - 1
		} else if timeDiff < BlockMiningInterval/2 {
			if prevBlock.Difficulty != 0 {
				return prevBlock.Difficulty + 1
			}
		}
	}
	return prevBlock.Difficulty
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

// dump single block
func (block *Block) PrintBlock() {
	fmt.Printf("\n**************************** BLOCK %d ****************************\n\n", block.Height+1)
	fmt.Printf("Block Header\n")
	fmt.Printf("\t|-Timestamp        : %v\n", time.Unix(block.Timestamp, 0))
	fmt.Printf("\t|-Nonce            : %#x\n", block.Nonce)
	fmt.Printf("\t|-Difficulty       : %d\n", block.Difficulty)
	fmt.Printf("\t|-Hash             : %x\n", block.Hash)
	fmt.Printf("\t|-PrevHash         : %x\n", block.PrevHash)
	fmt.Printf("\t|-Height           : %d\n", block.Height)

	fmt.Println("\nTransactions")
	for _, tx := range block.Txs {
		tx.PrintTx()
	}

	pow := NewProof(block)
	fmt.Printf("ValidBlock : %s\n", strconv.FormatBool(pow.Validate()))
	fmt.Printf("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++\n\n\n")
}
