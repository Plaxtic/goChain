package network

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"exx/gochain/blockchain"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
)

const (
	protocol   = "tcp"
	version    = 1
	commandLen = 12
)

var (
	nodeAddress     string
	mineAddress     string
	KnownNodes      = []string{}
	blocksInTransit = [][]byte{}
	memoryPool      = make(map[string]blockchain.Tx)
)

type Addr struct {
	AddrList []string
}

type Block struct {
	AddrFrom string
	Block    []byte
}

type GetBlocks struct {
	AddrFrom string
}

type GetData struct {
	AddrFrom string
	Type     string
	ID       []byte
}

type Inventory struct {
	AddrFrom string
	Type     string
	Items    [][]byte
}

type Tx struct {
	AddrFrom    string
	Transaction []byte
}

type Version struct { // remote procedure call (RPC)
	Version    int
	BestHeight int // to compare blockchain lengths
	AddrFrom   string
}

func Cmd2Bytes(cmd string) []byte {
	var bytes [commandLen]byte

	for i, c := range cmd {
		bytes[i] = byte(c)
	}
	return bytes[:]
}

func Bytes2Cmd(bytes []byte) string {
	var cmd []byte

	for _, b := range bytes {
		if b != 0x0 {
			cmd = append(cmd, b)
		}
	}
	return fmt.Sprintf("%s", cmd)
}

func GobEncode(data interface{}) []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	HandleErr(enc.Encode(data))

	return buff.Bytes()
}

func SendAddr(address string) {
	nodes := Addr{
		AddrList: KnownNodes,
	}
	nodes.AddrList = append(nodes.AddrList, nodeAddress)
	payload := GobEncode(nodes)
	request := append(Cmd2Bytes("addr"), payload...)

	SendData(address, request)
}

func SendBlock(address string, b *blockchain.Block) {
	data := Block{
		AddrFrom: nodeAddress,
		Block:    b.ToBytes(),
	}
	payload := GobEncode(data)
	request := append(Cmd2Bytes("block"), payload...)

	SendData(address, request)
}

func SendInv(address, kind string, items [][]byte) {
	data := Inventory{
		AddrFrom: nodeAddress,
		Type:     kind,
		Items:    items,
	}
	payload := GobEncode(data)
	request := append(Cmd2Bytes("inv"), payload...)

	SendData(address, request)
}

func SendTx(address string, tnx *blockchain.Tx) {
	data := Tx{
		AddrFrom:    nodeAddress,
		Transaction: tnx.ToBytes(),
	}
	payload := GobEncode(data)
	request := append(Cmd2Bytes("tx"), payload...)

	SendData(address, request)
}

func SendVersionAck(address string, chain *blockchain.BlockChain) {
	data := Version{
		Version:    version,
		BestHeight: chain.GetBestHeight(),
		AddrFrom:   nodeAddress,
	}
	payload := GobEncode(data)
	request := append(Cmd2Bytes("verack"), payload...)

	SendData(address, request)
}

func SendVersion(address string, chain *blockchain.BlockChain) {
	data := Version{
		Version:    version,
		BestHeight: chain.GetBestHeight(),
		AddrFrom:   nodeAddress,
	}
	payload := GobEncode(data)
	request := append(Cmd2Bytes("version"), payload...)

	SendData(address, request)
}

func SendGetBlocks(address string) {
	data := GetBlocks{
		AddrFrom: nodeAddress,
	}
	payload := GobEncode(data)
	request := append(Cmd2Bytes("getblocks"), payload...)

	SendData(address, request)
}

func SendGetData(address, kind string, id []byte) {
	data := GetData{
		AddrFrom: nodeAddress,
		Type:     kind,
		ID:       id,
	}
	payload := GobEncode(data)
	request := append(Cmd2Bytes("getdata"), payload...)

	SendData(address, request)
}

func SendData(address string, data []byte) {
	conn, err := net.Dial(protocol, address)

	if err != nil {
		var updatedNodes []string
		//		fmt.Printf("%s is unavailable\n", address)

		for _, node := range KnownNodes {
			if node != address {
				updatedNodes = append(updatedNodes, node)
			}
		}
		KnownNodes = updatedNodes

		return
	}
	defer conn.Close()

	_, err = io.Copy(conn, bytes.NewReader(data))
	HandleErr(err)
}

func HandleAddr(request []byte) {
	var buff bytes.Buffer
	var payload Addr

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

	KnownNodes = append(KnownNodes, payload.AddrList...)
	fmt.Printf("%2d known nodes\n", len(KnownNodes))
	RequestBlocks()
}

func HandleBlock(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Block

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

	blockData := payload.Block
	block := blockchain.Bytes2Block(blockData)
	chain.AddBlock(block)

	if len(blocksInTransit) > 0 {
		blockHash := blocksInTransit[0]
		SendGetData(payload.AddrFrom, "block", blockHash)

		blocksInTransit = blocksInTransit[1:]
	}
	UTXOst := blockchain.UTXOSet{
		BlockChain: chain,
	}
	UTXOst.Reindex()
}

func HandleGetBlocks(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetBlocks

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

	blocks := chain.GetHashes()
	SendInv(payload.AddrFrom, "block", blocks)
}

func HandleGetData(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetData

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

	switch payload.Type {
	case "block":
		block, err := chain.GetBlockByHash([]byte(payload.ID))
		if err != nil {
			return
		}
		SendBlock(payload.AddrFrom, &block)
	case "tx":
		txID := hex.EncodeToString(payload.ID)
		tx := memoryPool[txID]

		SendTx(payload.AddrFrom, &tx)
	default:
		fmt.Printf("Unrecognised data type: %s\n", payload.Type)
	}
}

func HandleVersionAck(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	// check if peer has longer blockchain
	if bestHeight < otherHeight {
		SendGetBlocks(payload.AddrFrom)
	}
	if NodeIsKnown(payload.AddrFrom) == false {
		KnownNodes = append(KnownNodes, payload.AddrFrom)
	}
}

func HandleVersion(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	// acknowledge peer
	SendVersionAck(payload.AddrFrom, chain)

	// check if peer has longer blockchain
	if bestHeight < otherHeight {
		SendGetBlocks(payload.AddrFrom)
	}

	// add node to known nodes
	if NodeIsKnown(payload.AddrFrom) == false {
		KnownNodes = append(KnownNodes, payload.AddrFrom)
	}
}

func HandleTx(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Tx

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

	txData := payload.Transaction
	tx := blockchain.Bytes2Tx(txData)
	memoryPool[hex.EncodeToString(tx.ID)] = tx

	fmt.Printf("%s, %d\n", nodeAddress, len(memoryPool))

	if nodeAddress == KnownNodes[0] {
		for _, node := range KnownNodes {
			if node != nodeAddress && node != payload.AddrFrom {
				SendInv(node, "tx", [][]byte{tx.ID})
			}
		}
	} else {
		if len(memoryPool) >= 2 && len(mineAddress) > 0 {
			MineTx(chain)
		}
	}
}

func HandleInv(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Inventory

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

	fmt.Printf("Recevied Inventory with %d %s\n", len(payload.Items), payload.Type)

	switch payload.Type {
	case "block":

		// refresh blocksInTransit
		for i, h := range payload.Items {
			if chain.ContainsBlock(h) {
				blocksInTransit = payload.Items[:i]
				break
			}
		}
		numHash := len(blocksInTransit)
		blockHash := blocksInTransit[numHash-1]
		SendGetData(payload.AddrFrom, "block", blockHash)

		newInTransit := [][]byte{}
		for i := numHash - 1; i >= 0; i-- {
			b := blocksInTransit[i]
			if bytes.Compare(b, blockHash) != 0 {
				newInTransit = append(newInTransit, b)
			}
		}
		blocksInTransit = newInTransit

	case "tx":
		txID := payload.Items[0]

		if memoryPool[hex.EncodeToString(txID)].ID == nil {
			SendGetData(payload.AddrFrom, "tx", txID)
		}
	}
}

func HandleConnection(conn net.Conn, chain *blockchain.BlockChain) {
	req, err := ioutil.ReadAll(conn)
	defer conn.Close()
	HandleErr(err)

	cmd := Bytes2Cmd(req[:commandLen])
	req = req[commandLen:]
	fmt.Printf("Received %s command\n", cmd)

	switch cmd {
	case "addr":
		HandleAddr(req)
	case "block":
		HandleBlock(req, chain)
	case "inv":
		HandleInv(req, chain)
	case "getblocks":
		HandleGetBlocks(req, chain)
	case "getdata":
		HandleGetData(req, chain)
	case "tx":
		HandleTx(req, chain)
	case "version":
		HandleVersion(req, chain)
	case "verack":
		HandleVersionAck(req, chain)
	default:
		fmt.Println("Unknown command")
	}
}

func MineTx(chain *blockchain.BlockChain) {
	var txs []*blockchain.Tx

	for id := range memoryPool {
		fmt.Printf("tx: %s\n", memoryPool[id].ID)
		tx := memoryPool[id]
		if chain.VerifyTx(&tx) {
			txs = append(txs, &tx)
		}
	}

	if len(txs) == 0 {
		fmt.Println("All Transactions are invalid")
		return
	}

	cbTx := blockchain.CoinbaseTx(mineAddress, "")
	txs = append(txs, cbTx)

	// refresh UTXOs
	newBlock := chain.MineBlock(txs)
	UTXOst := blockchain.UTXOSet{
		BlockChain: chain,
	}
	UTXOst.Reindex()
	fmt.Println("New block mined")

	for _, tx := range txs {
		txID := hex.EncodeToString(tx.ID)
		delete(memoryPool, txID)
	}

	for _, node := range KnownNodes {
		if node != nodeAddress {
			SendInv(node, "block", [][]byte{newBlock.Hash})
		}
	}

	if len(memoryPool) > 0 {
		MineTx(chain)
	}
}

func NodeIsKnown(address string) bool {
	for _, node := range KnownNodes {
		if node == address {
			return true
		}
	}
	return false
}

// helps to sync blockchains
func RequestBlocks() {
	for _, node := range KnownNodes {
		SendGetBlocks(node)
	}
}

func HandleErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}
