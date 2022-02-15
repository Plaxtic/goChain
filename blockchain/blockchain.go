package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dgraph-io/badger"
)

const (
	dbPath      = "./tmp/blocks_%s"
	genesisData = "GENESIS"
)

type BlockChain struct {
	LastHash []byte
	Database *badger.DB
}

// add block to chain
func (chain *BlockChain) MineBlock(txs []*Tx) *Block {
	var lastHash, lastBlockData []byte
	var lastHeight int

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(lastHash)
		Handle(err)

		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, err := item.ValueCopy(lastBlockData)

		lastBlock := Bytes2Block(lastBlockData)
		lastHeight = lastBlock.Height

		return err
	})
	Handle(err)

	newBlock := CreateBlock(txs, lastHash, lastHeight)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		err := txn.Set(newBlock.Hash, newBlock.ToBytes())
		Handle(err)
		err = txn.Set([]byte("lh"), newBlock.Hash)

		chain.LastHash = newBlock.Hash

		return err
	})
	Handle(err)

	return newBlock
}

func (chain *BlockChain) AddBlock(block *Block) {
	var lastHash []byte

	err := chain.Database.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(block.Hash); err == nil {
			return nil
		}

		blockData := block.ToBytes()
		Handle(txn.Set(block.Hash, blockData))

		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, _ := item.ValueCopy(lastHash)

		item, err = txn.Get(lastHash)
		lastBlockData, _ := item.ValueCopy(lastHash)

		lastBlock := Bytes2Block(lastBlockData)

		if block.Height > lastBlock.Height {
			Handle(txn.Set([]byte("lh"), block.Hash))
			chain.LastHash = block.Hash
		}
		return err
	})
	Handle(err)
}

func (chain *BlockChain) GetBlock(blockHash []byte) (Block, error) {
	var block Block

	err := chain.Database.View(func(txn *badger.Txn) error {
		if item, err := txn.Get(blockHash); err != nil {
			return errors.New("Block not found")
		} else {
			var blockData []byte

			blockData, _ = item.ValueCopy(blockData)
			block = *Bytes2Block(blockData)
		}
		return nil
	})
	return block, err
}

func (chain *BlockChain) GetHashes() [][]byte {
	var blocks [][]byte
	iter := chain.Iterator()

	for block, err := iter.Next(); err == nil; block, err = iter.Next() {
		blocks = append(blocks, block.Hash)
	}
	return blocks
}

func (chain *BlockChain) GetBestHeight() int {
	var lastBlock Block
	var lastHash, lastBlockData []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		Handle(err)
		lastHash, err = item.ValueCopy(lastHash)
		Handle(err)

		item, err = txn.Get(lastHash)
		Handle(err)
		lastBlockData, err := item.ValueCopy(lastBlockData)

		lastBlock = *Bytes2Block(lastBlockData)

		return nil
	})
	Handle(err)

	return lastBlock.Height
}

func DBexists(path string) bool {
	if _, err := os.Stat(path + "/MANIFEST"); os.IsNotExist(err) {
		return false
	}
	return true
}

// create or fetch stored blockchain
func InitBlockChain(address, nodeID string) *BlockChain {
	var lastHash []byte

	// if exists use ContinueBlockChain
	if DBexists(nodeID) {
		fmt.Println("Blockchain exists")
		runtime.Goexit()
	}

	path := fmt.Sprintf(dbPath, nodeID)
	opts := badger.DefaultOptions(path)
	opts.Dir = path
	opts.ValueDir = path

	db, err := badger.Open(opts)
	Handle(err)

	err = db.Update(func(txn *badger.Txn) error {
		cbtx := CoinbaseTx(address, genesisData)
		genesis := Genesis(cbtx)
		fmt.Println("Genesis Block minted")
		err = txn.Set(genesis.Hash, genesis.ToBytes())
		Handle(err)
		err = txn.Set([]byte("lh"), genesis.Hash)

		lastHash = genesis.Hash

		return err
	})
	Handle(err)

	blockchain := BlockChain{lastHash, db}
	return &blockchain
}

func ContinueBlockChain(nodeID string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeID)
	if DBexists(path) == false {
		fmt.Println("No existing blockchain")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions(path)
	opts.Dir = path
	opts.ValueDir = path

	db, err := openDB(path, opts)
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

func Bytes2Tx(data []byte) Tx {
	var tx Tx

	dec := gob.NewDecoder(bytes.NewReader(data))
	Handle(dec.Decode(&tx))

	return tx
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

func retry(dir string, originalOpts badger.Options) (*badger.DB, error) {
	lockPath := filepath.Join(dir, "LOCK")
	if err := os.Remove(lockPath); err != nil {
		return nil, fmt.Errorf(`removing "LOCK": %s`, err)
	}
	retryOpts := originalOpts
	retryOpts.Truncate = true
	db, err := badger.Open(retryOpts)
	return db, err
}

// stuff to prevent database corruption (like when a process locking it is killed)
func openDB(dir string, opts badger.Options) (*badger.DB, error) {
	if db, err := badger.Open(opts); err != nil {
		if strings.Contains(err.Error(), "LOCK") {
			if db, err := retry(dir, opts); err == nil {
				log.Println("database unlocked, value log truncated")
				return db, nil
			}
			log.Println("could not unlock database:", err)
		}
		return nil, err
	} else {
		return db, nil
	}
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
