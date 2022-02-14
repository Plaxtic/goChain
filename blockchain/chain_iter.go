package blockchain

import (
	"errors"

	"github.com/dgraph-io/badger"
)

type BlockChainIterator struct {
	CurrHash []byte
	Database *badger.DB
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

	// get next block
	var block *Block
	var encodedBlock []byte
	err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrHash)
		encodedBlock, err := item.ValueCopy(encodedBlock)
		block = Bytes2Block(encodedBlock)

		return err
	})
	Handle(err)

	// step back one block
	iter.CurrHash = block.PrevHash
	return block, nil
}
