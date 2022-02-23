package blockchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	log "github.com/llimllib/loglevel"
)

const (
	InitialDifficulty   = 15
	BlockMiningInterval = 10
	AdjustmentInverval  = 5
)

type ProofOfWork struct {
	Block  *Block
	Target *big.Int
}

func NewProof(b *Block) *ProofOfWork {

	// left bitshift
	target := big.NewInt(1)
	target.Lsh(target, uint(256-b.Difficulty))

	pow := &ProofOfWork{b, target}

	return pow
}

func (pow *ProofOfWork) InitData(nonce int) []byte {
	data := bytes.Join(
		[][]byte{
			ToBytes(int64(nonce)),
			ToBytes(pow.Block.Timestamp),
			ToBytes(int64(pow.Block.Difficulty)),
			pow.Block.PrevHash,
			ToBytes(int64(pow.Block.Height)),

			// pow.Block.HashTxs(),
		},
		[]byte{},
	)
	return data
}

func (pow *ProofOfWork) Run() (int, []byte) {
	var intHash big.Int
	var hash [32]byte

	// brute force hash
	nonce := 0
	for nonce < math.MaxInt64 {
		data := pow.InitData(nonce)
		hash = sha256.Sum256(data)

		fmt.Printf("\r%x", hash)
		intHash.SetBytes(hash[:])

		// check matches difficulty
		if intHash.Cmp(pow.Target) == -1 {
			break
		} else {
			nonce++
		}
	}
	fmt.Println()

	return nonce, hash[:]
}

// not working for excess zeros 				<--  (CHANGE)
func (pow *ProofOfWork) Validate() bool {
	var intHash big.Int
	var hash [32]byte

	data := pow.InitData(pow.Block.Nonce)
	hash = sha256.Sum256(data)

	intHash.SetBytes(hash[:])

	return intHash.Cmp(pow.Target) == -1
}

func ToBytes(num int64) []byte {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}
	return buf.Bytes()
}
