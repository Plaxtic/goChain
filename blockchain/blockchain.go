package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/dgraph-io/badger"
)

const (
	dbPath      = "./tmp/blocks"
	dbFile      = "./tmp/blocks/MANIFEST"
	genesisData = "GENESIS"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

type BlockChainIterator struct {
	CurrHash []byte
	Database *badger.DB
}

// add block to chain
func (chain *BlockChain) AddBlock(txs []*Tx) *Block {
	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(lastHash)

		return err
	})
	Handle(err)

	newBlock := CreateBlock(txs, lastHash)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.Block2Bytes())
		Handle(err)
		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	})
	Handle(err)

	return newBlock
}

func DBexists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

// create or fetch stored blockchain
func InitBlockChain(address string) *BlockChain {
	var lastHash []byte

	// if exists use ContinueBlockChain
	if DBexists() {
		fmt.Println("Blockchain exists")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		cbtx := CoinbaseTx(address, genesisData)
		genesis := Genesis(cbtx)
		fmt.Println("Genesis Block minted")
		err = txn.Set(genesis.Hash, genesis.Block2Bytes())
		Handle(err)
		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err
	})
	Handle(err)

	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

func ContinueBlockChain(address string) *BlockChain {
	if DBexists() == false {
		fmt.Println("No existing blockchain")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions(dbPath)
	opts.Dir = dbPath
	opts.ValueDir = dbPath

	db, err := badger.Open(opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(lastHash)

		return err
	})
	Handle(err)

	chain := BlockChain{lastHash, db}

	return &chain
}

func (chain *BlockChain) FindUnspentTxs(pubKeyHash []byte) []Tx {
	var unspentTxs []Tx

	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block, _ := iter.Next()
		if block == nil {
			break
		}

		for _, tx := range block.Txs {
			txID := hex.EncodeToString(tx.ID)

			// for continue
		Outputs:
			for outIdx, out := range tx.Outputs {
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				if out.IsLockedWithKey(pubKeyHash) {
					unspentTxs = append(unspentTxs, *tx)
				}
			}
			if tx.IsCoinbase() == false {
				for _, in := range tx.Inputs {
					if in.UsesKey(pubKeyHash) {
						inTxID := hex.EncodeToString(in.ID)
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
					}
				}
			}
		}
	}
	return unspentTxs
}

func (chain *BlockChain) FindUTXO() map[string]TxOutputs {
	UTXO := make(map[string]TxOutputs)
	spentTXOs := make(map[string][]int)

	iter := chain.Iterator()

	for blk, err := iter.Next(); err == nil; blk, err = iter.Next() {

		for _, tx := range blk.Txs {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}
				outs := UTXO[txID]
				outs.Outputs = append(outs.Outputs, out)
				UTXO[txID] = outs
			}
			if tx.IsCoinbase() == false {
				for _, in := range tx.Inputs {
					inTxID := hex.EncodeToString(in.ID)
					spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Out)
				}
			}
		}
	}
	return UTXO
}

func (chain *BlockChain) Iterator() *BlockChainIterator {
	iter := &BlockChainIterator{chain.LastHash, chain.Database}

	return iter
}

func (chain *BlockChain) FindTx(ID []byte) (Tx, error) {
	iter := chain.Iterator()

	for blk, err := iter.Next(); err == nil; blk, err = iter.Next() {
		for _, tx := range blk.Txs {
			if bytes.Compare(tx.ID, ID) == 0 {
				return *tx, nil
			}
		}
	}
	return Tx{}, errors.New("Transaction does not exist")
}

func (chain *BlockChain) SignTx(tx *Tx, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Tx)

	for _, input := range tx.Inputs {
		prevTX, err := chain.FindTx(input.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

func (chain *BlockChain) VerifyTx(tx *Tx) bool {
	if tx.IsCoinbase() {
		return true
	}

	prevTXs := make(map[string]Tx)

	for _, input := range tx.Inputs {
		prevTX, err := chain.FindTx(input.ID)
		Handle(err)
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	return tx.Verify(prevTXs)
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

// print blockchain block by block
func (chain *BlockChain) PrintBlockChain() {
	iterChain := chain.Iterator()

	for {
		block, err := iterChain.Next()
		if err != nil {
			return
		}
		fmt.Println("-------------------------------------------------------------------------------------")
		block.PrintBlock()
	}
}
