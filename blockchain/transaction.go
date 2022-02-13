package blockchain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"exx/gochain/wallet"
	"fmt"
	"log"
	"math/big"
)

type Tx struct {
	ID      []byte
	Inputs  []TxIn
	Outputs []TxOut
}

func (tx *Tx) Tx2Bytes() []byte {
	var encoded bytes.Buffer

	enc := gob.NewEncoder(&encoded)
	Handle(enc.Encode(tx))

	return encoded.Bytes()
}

func (tx *Tx) Hash() []byte {
	var hash [32]byte

	txCopy := *tx
	txCopy.ID = []byte{}

	hash = sha256.Sum256(txCopy.Tx2Bytes())

	return hash[:]
}

func (tx *Tx) Sign(privKey ecdsa.PrivateKey, prevTXs map[string]Tx) {
	if tx.IsCoinbase() {
		return
	}

	for _, input := range tx.Inputs {
		if prevTXs[hex.EncodeToString(input.ID)].ID == nil {
			log.Panic("ERROR: Previous transaction is invalid")
		}
	}

	txCopy := tx.TrimmedCopy()

	for inId, input := range txCopy.Inputs {
		prevTX := prevTXs[hex.EncodeToString(input.ID)]
		txCopy.Inputs[inId].Sig = nil
		txCopy.Inputs[inId].PubKey = prevTX.Outputs[input.Out].PublicKeyHash
		txCopy.ID = txCopy.Hash()
		txCopy.Inputs[inId].PubKey = nil

		r, s, err := ecdsa.Sign(rand.Reader, &privKey, txCopy.ID)
		Handle(err)
		signature := append(r.Bytes(), s.Bytes()...)

		tx.Inputs[inId].Sig = signature
	}
}

func (tx *Tx) TrimmedCopy() Tx {
	var inputs []TxIn
	var outputs []TxOut

	for _, input := range tx.Inputs {
		inputs = append(inputs, TxIn{input.ID, input.Out, nil, nil})
	}
	for _, out := range tx.Outputs {
		outputs = append(outputs, TxOut{out.Value, out.PublicKeyHash})
	}

	txCopy := Tx{tx.ID, inputs, outputs}

	return txCopy
}

func (tx *Tx) Verify(prevTXs map[string]Tx) bool {
	if tx.IsCoinbase() {
		return true
	}

	txCopy := tx.TrimmedCopy()
	curve := elliptic.P256()

	// I CHANGED THIS MIGHT BE WRONG
	for inId, input := range tx.Inputs {
		if prevTXs[hex.EncodeToString(input.ID)].ID == nil {
			log.Panic("Previous transaction does not exist")
		}

		prevTx := prevTXs[hex.EncodeToString(input.ID)]
		txCopy.Inputs[inId].Sig = nil
		txCopy.Inputs[inId].PubKey = prevTx.Outputs[input.Out].PublicKeyHash
		txCopy.ID = txCopy.Hash()
		txCopy.Inputs[inId].PubKey = nil

		r := big.Int{}
		s := big.Int{}
		sigLen := len(input.Sig)
		r.SetBytes(input.Sig[:(sigLen / 2)])
		s.SetBytes(input.Sig[(sigLen / 2):])

		x := big.Int{}
		y := big.Int{}
		keyLen := len(input.PubKey)
		x.SetBytes(input.PubKey[:(keyLen / 2)])
		y.SetBytes(input.PubKey[(keyLen / 2):])

		rawPubKey := ecdsa.PublicKey{Curve: curve, X: &x, Y: &y} // Nouveau-syntax
		if ecdsa.Verify(&rawPubKey, txCopy.ID, &r, &s) == false {
			return false
		}
	}
	return true
}

func CoinbaseTx(to, data string) *Tx {
	if data == "" {
		data = fmt.Sprintf("Coins to %s", to)
	}

	txin := TxIn{[]byte{}, 01, nil, []byte(data)}
	txout := NewTxOut(100, to) // 100 coin block minting reward

	tx := Tx{nil, []TxIn{txin}, []TxOut{*txout}}
	tx.SetID()

	return &tx
}

func (tx *Tx) SetID() {
	var encoded bytes.Buffer
	var hash [32]byte

	encode := gob.NewEncoder(&encoded)
	err := encode.Encode(tx)
	Handle(err)

	hash = sha256.Sum256(encoded.Bytes())
	tx.ID = hash[:]
}

func NewTx(from, to string, amount int, chain *BlockChain) *Tx {
	var inputs []TxIn
	var outputs []TxOut

	// add wallets and retrive hash
	wallets, err := wallet.CreateWallets()
	Handle(err)
	w := wallets.GetWallet(from)
	pubKeyHash := wallet.PublicKeyHash(w.PublicKey)

	acc, validOutputs := chain.FindSpendableOutputs(pubKeyHash, amount)
	if acc < amount {
		log.Panic("Error: insufficient funds")
	}

	for txid, outs := range validOutputs {
		txID, err := hex.DecodeString(txid)
		Handle(err)

		for _, out := range outs {
			input := TxIn{txID, out, nil, w.PublicKey}
			inputs = append(inputs, input)
		}
	}
	outputs = append(outputs, *NewTxOut(amount, to))

	// if balance is not zero we need to create a transaction to ourself
	if acc > amount {
		outputs = append(outputs, *NewTxOut(amount-amount, to))
	}

	tx := Tx{nil, inputs, outputs}
	tx.Hash()
	chain.SignTx(&tx, w.PrivateKey)

	return &tx
}

func (tx *Tx) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && len(tx.Inputs[0].ID) == 0 && tx.Inputs[0].Out == -1
}

func (tx *Tx) PrintTx() {
	fmt.Printf("  TX ID: %x\n", tx.ID)

	for i, in := range tx.Inputs {
		fmt.Printf("  intput %d:\n", i+1)
		fmt.Printf("    ID         :  %x\n", in.ID)
		fmt.Printf("    OUT        :  %d\n", in.Out)
		fmt.Printf("    Signature  :  %x\n", in.Sig)
	}
	for i, out := range tx.Outputs {
		fmt.Printf("  Output %d:\n", i+1)
		fmt.Printf("    Value      :  %d\n", out.Value)
		fmt.Printf("    Public Key :  %x\n", out.PublicKeyHash)
	}
}
