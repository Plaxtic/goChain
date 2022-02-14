package wallet

import (
	"bytes"
	"crypto/elliptic"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
)

const walletPath = "./tmp/wallets_%s.data"

type Wallets struct {
	Wallets map[string]*Wallet
}

func CreateWallets(nodeID string) (*Wallets, error) {
	wallets := Wallets{}
	wallets.Wallets = make(map[string]*Wallet)

	err := wallets.LoadFile(nodeID)

	return &wallets, err
}

func (ws *Wallets) GetWallet(address string) Wallet {
	w := *ws.Wallets[address]
	return w
}

func (ws *Wallets) GetAllAddresses() []string {
	var addresses []string
	for addr := range ws.Wallets {
		addresses = append(addresses, addr)
	}
	return addresses
}

func (ws *Wallets) AddWallet() string {
	wallet := MakeWallet()
	address := fmt.Sprintf("%s", wallet.GetAddress())

	ws.Wallets[address] = wallet

	return address
}

func (ws *Wallets) LoadFile(nodeID string) error {
	walletFile := fmt.Sprintf(walletPath, nodeID)
	if _, err := os.Stat(walletFile); os.IsNotExist(err) {
		return err
	}

	var wallets Wallets

	fileContent, err := ioutil.ReadFile(walletFile)
	if err != nil {
		return err
	}

	gob.Register(elliptic.P256())
	decoder := gob.NewDecoder(bytes.NewReader(fileContent))
	err = decoder.Decode(&wallets)
	if err != nil {
		return err
	}

	ws.Wallets = wallets.Wallets

	return nil
}

func (ws *Wallets) SaveFile(nodeID string) {
	var content bytes.Buffer
	walletFile := fmt.Sprintf(walletPath, nodeID)

	gob.Register(elliptic.P256())

	encoder := gob.NewEncoder(&content)
	Handle(encoder.Encode(ws))
	Handle(ioutil.WriteFile(walletFile, content.Bytes(), 0644))
}
