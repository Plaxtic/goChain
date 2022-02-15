package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	dbPath      = "./tmp/blocks_%s"
	genesisData = "GENESIS"
)

type BlockChain struct {
	LastHash []byte
	Database *leveldb.DB
}

// create and store blockchain
func InitBlockChain(address, nodeID string) *BlockChain {
	var lastHash []byte

	// if exists use ContinueBlockChain
	if DBexists(nodeID) {
		fmt.Println("Blockchain exists")
		runtime.Goexit()
	}

	// open leveldb
	path := fmt.Sprintf(dbPath, nodeID)
	db, err := leveldb.OpenFile(path, nil)
	HandleErr(err)

	// mine genesis block
	cbtx := CoinbaseTx(address, genesisData)
	genesis := Genesis(cbtx)
	fmt.Println("Genesis Block minted")

	// create blockchain
	lastHash = genesis.Hash
	newBlockchain := BlockChain{
		LastHash: lastHash,
		Database: db,
	}
	newBlockchain.AddBlock(genesis)

	return &newBlockchain
}

func (chain *BlockChain) AddBlock(block *Block) {

	// reference block by hash
	HandleErr(chain.Database.Put(block.Hash, block.ToBytes(), nil))

	// delete previous head
	HandleErr(chain.Database.Delete([]byte("lh"), nil))

	// refernce hash by "lh" (last-hash)
	HandleErr(chain.Database.Put([]byte("lh"), block.Hash, nil))

	// update head
	chain.LastHash = block.Hash
	fmt.Printf("Added block %x\n", block.Hash)
}

func (chain *BlockChain) MineBlock(txs []*Tx) *Block {

	// get latest block height
	lastBlock := chain.GetLastBlock()
	lastHeight := lastBlock.Height

	// create new block
	newBlock := CreateBlock(txs, lastBlock.Hash, lastHeight+1)

	// add new block to chain
	chain.AddBlock(newBlock)
	chain.LastHash = newBlock.Hash

	return newBlock
}

func (chain *BlockChain) GetLastBlock() Block {

	// get last hash by refernce
	lastHash, err := chain.Database.Get([]byte("lh"), nil)
	HandleErr(err)

	// get block by hash
	lastBlock, err := chain.GetBlockByHash(lastHash)
	HandleErr(err)

	return lastBlock
}

func (chain *BlockChain) SelectChain(block *Block) bool {

	// if new block is longer, update chain
	if block.Height > chain.GetLastBlock().Height {
		HandleErr(chain.Database.Put([]byte("lh"), block.Hash, nil))
		chain.LastHash = block.Hash
		return true
	}
	return false
}

func (chain *BlockChain) GetBlockByHash(blockHash []byte) (Block, error) {
	var block Block

	// get raw block data
	lastBlockData, err := chain.Database.Get(blockHash, nil)
	if err != nil {
		return block, err
	}

	// decode block
	block = *Bytes2Block(lastBlockData)

	return block, nil
}

func (chain *BlockChain) GetHashes() [][]byte {
	var hashes [][]byte
	iter := chain.Iterator()

	for block, err := iter.Next(); err == nil; block, err = iter.Next() {
		hashes = append(hashes, block.Hash)
	}
	return hashes
}

func (chain *BlockChain) GetBestHeight() int {
	return chain.GetLastBlock().Height
}

func DBexists(path string) bool {
	if _, err := os.Stat(path + "/CURRENT"); os.IsNotExist(err) {
		return false
	}
	return true
}

func ContinueBlockChain(nodeID string) *BlockChain {
	path := fmt.Sprintf(dbPath, nodeID)
	if DBexists(path) == false {
		fmt.Println("No existing blockchain")
		runtime.Goexit()
	}

	db, err := leveldb.OpenFile(path, nil)
	HandleErr(err)

	// create blockchain
	chain := BlockChain{
		LastHash: []byte{},
		Database: db,
	}

	// get last hash from latest block
	chain.LastHash = chain.GetLastBlock().Hash

	return &chain
}

func (chain *BlockChain) FindUnspentTxs(pubKeyHash []byte) []Tx {
	var unspentTxs []Tx
	spentTXOs := make(map[string][]int)

	// create blockchain iterator
	iter := chain.Iterator()

	// iterate through blocks
	for block, err := iter.Next(); err == nil; block, err = iter.Next() {
		for _, tx := range block.Txs {
			txID := hex.EncodeToString(tx.ID)

			// iterate each transaction output
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
		HandleErr(err)
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
		HandleErr(err)
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

	for block, err := iterChain.Next(); err == nil; block, err = iterChain.Next() {
		fmt.Println("-------------------------------------------------------------------------------------")
		block.PrintBlock()
	}
}
