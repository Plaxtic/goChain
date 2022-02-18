package blockchain

import (
	"bytes"
	"encoding/hex"

	"github.com/dgraph-io/badger"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	utxoPrefix   = []byte("utxo-")
	prefixLength = len(utxoPrefix)
)

type UTXOSet struct {
	BlockChain *BlockChain
}

func (u UTXOSet) FindSpendableOutputs(pubKeyHash []byte, amount int) (int, map[string][]int) {
	unspentOuts := make(map[string][]int)
	accumulated := 0

	// create new iterator
	db := u.BlockChain.Database
	it := db.NewIterator(util.BytesPrefix(utxoPrefix), nil)
	defer it.Release()

	for it.Next() {
		k := it.Key()
		v := it.Value()
		k = bytes.TrimPrefix(k, utxoPrefix)

		txID := hex.EncodeToString(k)
		outs := Bytes2Txoutputs(v)

		for outIdx, out := range outs.Outputs {
			if out.IsLockedWithKey(pubKeyHash) && accumulated < amount {
				accumulated += out.Value
				unspentOuts[txID] = append(unspentOuts[txID], outIdx)
			}
		}
	}
	return accumulated, unspentOuts
}

func (u UTXOSet) CountTransactions() int {
	db := u.BlockChain.Database
	counter := 0

	it := db.NewIterator(util.BytesPrefix(utxoPrefix), nil)
	defer it.Release()
	for it.Next() {
		counter++
	}

	return counter
}

func (u UTXOSet) FindUTXO(pubKeyHash []byte) []TxOut {
	var UTXOs []TxOut

	// create leveldb iterator
	db := u.BlockChain.Database
	it := db.NewIterator(util.BytesPrefix(utxoPrefix), nil)
	defer it.Release()

	// iterate through values of prefix "utxoPrefix"
	for it.Next() {

		v := it.Value()
		outs := Bytes2Txoutputs(v)

		for _, out := range outs.Outputs {
			if out.IsLockedWithKey(pubKeyHash) {
				UTXOs = append(UTXOs, out)
			}
		}
	}
	return UTXOs
}
func (u UTXOSet) Reindex() {
	db := u.BlockChain.Database

	u.DeleteByPrefix(utxoPrefix)
	UTXO := u.BlockChain.FindUTXO()

	// refresh database
	for txId, outs := range UTXO {
		key, err := hex.DecodeString(txId)
		HandleErr(err)
		key = append(utxoPrefix, key...)

		err = db.Put(key, outs.ToBytes(), nil)
		HandleErr(err)
	}
}

func (u *UTXOSet) Update(block *Block) {
	db := u.BlockChain.Database

	for _, tx := range block.Txs {
		if tx.IsCoinbase() == false {
			for _, in := range tx.Inputs {
				updatedOuts := TxOutputs{}
				inID := append(utxoPrefix, in.ID...)

				value, err := db.Get(inID, nil) // problem
				//				if strings.Contains(err.Error(), "leveldb: not found")
				HandleErr(err)
				outs := Bytes2Txoutputs(value)

				for outIdx, out := range outs.Outputs {
					if outIdx != in.Out {
						updatedOuts.Outputs = append(updatedOuts.Outputs, out)
					}
				}
				if len(updatedOuts.Outputs) == 0 {
					HandleErr(db.Delete(inID, nil))
				} else {
					HandleErr(db.Put(inID, updatedOuts.ToBytes(), nil))
				}
			}
		}

		newOutputs := TxOutputs{}
		for _, out := range tx.Outputs {
			newOutputs.Outputs = append(newOutputs.Outputs, out)
		}

		txID := append(utxoPrefix, tx.ID...)
		HandleErr(db.Put(txID, newOutputs.ToBytes(), nil))
	}
}

func (u *UTXOSet) DeleteByPrefix(prefix []byte) {
	db := u.BlockChain.Database

	deleteKeys := func(keysForDelete [][]byte) error {
		for _, key := range keysForDelete {
			if err := db.Delete(key, nil); err != nil {
				return err
			}
		}
		return nil
	}

	collectSize := 100000
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false

	it := db.NewIterator(util.BytesPrefix(prefix), nil)
	defer it.Release()

	keysForDelete := make([][]byte, 0, collectSize)
	keysCollected := 0
	for it.Next() {
		key := it.Key()
		keysForDelete = append(keysForDelete, key)
		keysCollected++

		if keysCollected == collectSize {
			HandleErr(deleteKeys(keysForDelete))
			keysForDelete = make([][]byte, 0, collectSize)
			keysCollected = 0
		}
	}
	if keysCollected > 0 {
		HandleErr(deleteKeys(keysForDelete))
	}
}
