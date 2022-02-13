package wallet

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/ripemd160"
)

const (
	checksumLen = 4
	version     = byte(0x00)
)

type Wallet struct {
	PrivateKey ecdsa.PrivateKey
	PublicKey  []byte
}

func NewKeyPair() (ecdsa.PrivateKey, []byte) {
	curve := elliptic.P256()

	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	Handle(err)

	public := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...) // ????
	return *private, public
}

// mad cryptography otw
func PublicKeyHash(pubKey []byte) []byte {
	pubHash := sha256.Sum256(pubKey)

	// ripe hashes the public key again, but outputs only 160 bits
	hasher := ripemd160.New()
	_, err := hasher.Write(pubHash[:])
	Handle(err)

	publicRipMD := hasher.Sum(nil)

	return publicRipMD
}

// this takes the output of the PublicKeyHash(), hashes it twice more and grabs the last 4 bytes
// fuck knows why
func Checksum(input []byte) []byte {
	firstHash := sha256.Sum256(input)
	secondHash := sha256.Sum256(firstHash[:])

	return secondHash[:checksumLen]
}

func (w Wallet) GetAddress() []byte {

	// hash public key (twice)
	pubHash := PublicKeyHash(w.PublicKey)

	// concatenate with version
	versionedHash := append([]byte{version}, pubHash...)

	// hash twice more and grab first 4 bytes
	checksum := Checksum(versionedHash)

	// concatenate with four byte checksum
	fullHash := append(versionedHash, checksum...)

	// finaly, encode in base58 to get address
	address := Base58Encode(fullHash)

	fmt.Printf("Public Key  : %x\n", w.PublicKey)
	fmt.Printf("Public Hash : %x\n", pubHash)
	fmt.Printf("Address     : %x\n", address)

	return address
}
func ValidateAddress(address string) bool {
	pubKeyHash := Base58Decode([]byte(address))
	actualChecksum := pubKeyHash[len(pubKeyHash)-checksumLen+1:]
	version := pubKeyHash[0]
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-checksumLen+1]
	targetChecksum := Checksum(append([]byte{version}, pubKeyHash...))

	return bytes.Compare(actualChecksum, targetChecksum) == 0
}

func MakeWallet() *Wallet {
	private, public := NewKeyPair()
	wallet := Wallet{private, public}

	return &wallet
}
