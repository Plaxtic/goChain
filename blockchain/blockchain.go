package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
)

const (
	DBPath      = "./tmp/blocks_%s"
	genesisData = "GENESIS"
)

type BlockChain struct {
	Database *leveldb.DB
}

// create and store blockchain
func (chain *BlockChain) CreateBlockChain(address, nodeID string) {

	// check for existing blockchain
	_, err := chain.GetLastBlock()
	if err == nil {
		log.Panic("Chain already exists")
	}

	// mine genesis block
	cbtx := CoinbaseTx(address, genesisData)
	genesis := Genesis(cbtx)
	fmt.Println("Genesis Block minted")

	// create blockchain
	chain.AddBlock(genesis)
}

func (chain *BlockChain) ContainsBlock(hash []byte) bool {
	iter := chain.Iterator()

	for iter.Next() {
		if bytes.Compare(iter.Block.Hash, hash) == 0 {
			return true
		}
	}
	return false
}

func (chain *BlockChain) AddBlock(block *Block) {

	// delete previous head
	HandleErr(chain.Database.Delete([]byte("lh"), nil))

	// reference block by hash
	HandleErr(chain.Database.Put(block.Hash, block.ToBytes(), nil))

	// refernce hash by "lh" (last-hash)
	HandleErr(chain.Database.Put([]byte("lh"), block.Hash, nil))
}

func (chain *BlockChain) MineBlock(txs []*Tx) *Block {

	// get latest block height
	lastBlock, err := chain.GetLastBlock()
	HandleErr(err)
	lastHeight := lastBlock.Height

	// create new block
	newBlock := CreateBlock(txs, lastBlock.Hash, lastHeight+1)

	// add new block to chain
	chain.AddBlock(newBlock)

	return newBlock
}

func (chain *BlockChain) GetLastBlock() (lastBlock Block, err error) {

	// get last hash by refernce
	lastHash, err := chain.Database.Get([]byte("lh"), nil)
	if err != nil {
		return lastBlock, err
	}

	// get block by hash
	lastBlock, err = chain.GetBlockByHash(lastHash)
	HandleErr(err)
	return lastBlock, nil
}

func (chain *BlockChain) SelectChain(block *Block) bool {

	// if new block is longer, update chain
	lastBlock, err := chain.GetLastBlock()
	HandleErr(err)
	if block.Height > lastBlock.Height {
		HandleErr(chain.Database.Put([]byte("lh"), block.Hash, nil))
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

	for iter.Next() {
		hashes = append(hashes, iter.Block.Hash)
	}
	return hashes
}

func (chain *BlockChain) GetBestHeight() int {
	lastBlock, err := chain.GetLastBlock()
	if err != nil {
		return 0
	}

	return lastBlock.Height
}

func DBexists(path string) bool {
	if _, err := os.Stat(path + "/CURRENT"); os.IsNotExist(err) {
		return false
	}
	return true
}

func ContinueBlockChain(nodeID string) *BlockChain {

	// open database
	path := fmt.Sprintf(DBPath, nodeID)
	db, err := leveldb.OpenFile(path, nil)
	HandleErr(err)

	// pass database to structure
	chain := BlockChain{
		Database: db,
	}

	return &chain
}

func (chain *BlockChain) FindUnspentTxs(pubKeyHash []byte) []Tx {
	var unspentTxs []Tx
	spentTXOs := make(map[string][]int)

	// create blockchain iterator
	iter := chain.Iterator()

	// iterate through blocks
	for iter.Next() {
		for _, tx := range iter.Block.Txs {
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

	for iter.Next() {
		for _, tx := range iter.Block.Txs {
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

	for iter.Next() {
		for _, tx := range iter.Block.Txs {
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

// print blockchain block by block
func (chain *BlockChain) PrintBlockChain() {
	iter := chain.Iterator()

	for iter.Next() {
		iter.Block.PrintBlock()
	}
}
