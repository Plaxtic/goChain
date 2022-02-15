package blockchain

import (
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
)

type BlockChainIterator struct {
	CurrHash []byte
	Database *leveldb.DB
}

func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}

	return iter
}

func (iter *BlockChainIterator) Next() (*Block, error) {

	// check more blocks
	if iter.CurrHash == nil {
		return nil, errors.New("StopIteration")
	}

	// get the current block
	blockData, err := iter.Database.Get(iter.CurrHash, nil)
	HandleErr(err)
	block := Bytes2Block(blockData)

	// step back one block
	iter.CurrHash = block.PrevHash
	return block, nil
}
