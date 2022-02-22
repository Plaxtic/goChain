package blockchain

import (
	"github.com/syndtr/goleveldb/leveldb"
)

type BlockChainIterator struct {
	CurrHash []byte
	Block    *Block
	Database *leveldb.DB
}

func (chain *BlockChain) Iterator() *BlockChainIterator {
	var currHash []byte

	lastBlock, err := chain.GetLastBlock()
	if err == nil {
		currHash = lastBlock.Hash
	} else {
		currHash = nil
	}

	iter := &BlockChainIterator{
		CurrHash: currHash,
		Block:    &lastBlock,
		Database: chain.Database,
	}

	return iter
}

func (iter *BlockChainIterator) Next() bool {

	// check more blocks
	if iter.CurrHash == nil {
		return false
	}

	// get the current block
	blockData, err := iter.Database.Get(iter.CurrHash, nil)
	HandleErr(err)
	currBlock := Bytes2Block(blockData)

	// step back one block
	iter.Block = currBlock
	iter.CurrHash = currBlock.PrevHash
	return true
}
