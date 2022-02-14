package blockchain

import (
	"bytes"
	"encoding/gob"
	"exx/gochain/wallet"
)

type TxOut struct {
	Value         int
	PublicKeyHash []byte
}

type TxIn struct {
	ID     []byte
	Out    int
	Sig    []byte
	PubKey []byte
}

type TxOutputs struct {
	Outputs []TxOut
}

func NewTxOut(value int, address string) *TxOut {
	txo := &TxOut{value, nil}
	txo.Lock([]byte(address))

	return txo
}

func (in *TxIn) UsesKey(pubKeyHash []byte) bool {
	lockingHash := wallet.PublicKeyHash(in.PubKey)

	return bytes.Compare(lockingHash, pubKeyHash) == 0
}

func (out *TxOut) Lock(address []byte) {
	pubKeyHash := wallet.Base58Decode(address)
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4] // remove version and checksum bytes
	out.PublicKeyHash = pubKeyHash
}

func (out *TxOut) IsLockedWithKey(pubKeyHash []byte) bool {
	return bytes.Compare(out.PublicKeyHash, pubKeyHash) == 0
}

func (outs *TxOutputs) ToBytes() []byte {
	var buffer bytes.Buffer

	encode := gob.NewEncoder(&buffer)
	Handle(encode.Encode(outs))

	return buffer.Bytes()
}

func Bytes2Txoutputs(data []byte) TxOutputs {
	var outputs TxOutputs
	decoder := gob.NewDecoder(bytes.NewReader(data))
	Handle(decoder.Decode(&outputs))
	return outputs
}
