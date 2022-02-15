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
	"os"
	"runtime"
	"strconv"
	"syscall"

	death "github.com/vrecan/death/v3" // like signal.h include
)

const (
	protocol   = "tcp"
	version    = 1
	commandLen = 12
)

var (
	nodeAddress     string
	mineAddress     string
	KnownNodes      = []string{"localhost:3000"}
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
	Handle(enc.Encode(data))

	return buff.Bytes()
}

func CloseDB(chain *blockchain.BlockChain) {
	d := death.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	d.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		chain.Database.Close()
	})
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

func SendVersion(address string, chain *blockchain.BlockChain) {
	data := Version{
		Version:    version,
		BestHeight: chain.GetBestHeight(),
		AddrFrom:   nodeAddress,
	}
	payload := GobEncode(data)
	request := append(Cmd2Bytes("version"), payload...)

	//HexDump(request)

	SendData(address, request)
}

func SendGetBlocks(address string) {
	data := GetBlocks{
		AddrFrom: nodeAddress,
	}
	payload := GobEncode(data)
	request := append(Cmd2Bytes("getdata"), payload...)

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

func SendData(addr string, data []byte) {
	conn, err := net.Dial(protocol, addr)

	if err != nil {
		var updatedNodes []string
		fmt.Printf("%s is unavailable\n", addr)

		for _, node := range KnownNodes {
			if node != addr {
				updatedNodes = append(updatedNodes, node)
			}
		}
		KnownNodes = updatedNodes

		return
	}
	defer conn.Close()

	_, err = io.Copy(conn, bytes.NewReader(data))
	Handle(err)
}

func HandleAddr(request []byte) {
	var buff bytes.Buffer
	var payload Addr

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	Handle(dec.Decode(&payload))

	KnownNodes = append(KnownNodes, payload.AddrList...)
	fmt.Printf("%2d known nodes\n", len(KnownNodes))
	RequestBlocks()
}

func HandleBlock(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Block

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	Handle(dec.Decode(&payload))

	blockData := payload.Block
	block := blockchain.Bytes2Block(blockData)
	fmt.Println("Recevied block")
	chain.AddBlock(block)

	if len(blocksInTransit) > 0 {
		blockHash := blocksInTransit[0]
		SendGetData(payload.AddrFrom, "block", blockHash)

		blocksInTransit = blocksInTransit[1:]
	} else {
		UTXOst := blockchain.UTXOSet{
			BlockChain: chain,
		}
		UTXOst.Reindex()
	}
}

func HandleGetBlocks(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetBlocks

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	Handle(dec.Decode(&payload))

	blocks := chain.GetHashes()
	SendInv(payload.AddrFrom, "block", blocks)
}

func HandleGetData(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetData

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	Handle(dec.Decode(&payload))

	switch payload.Type {
	case "block":
		block, err := chain.GetBlock([]byte(payload.ID))
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

func HandleVersion(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Version

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	Handle(dec.Decode(&payload))

	bestHeight := chain.GetBestHeight()
	otherHeight := payload.BestHeight

	// check if peer has longer blockchain
	if bestHeight < otherHeight {
		SendGetBlocks(payload.AddrFrom)
	} else if bestHeight > otherHeight { // send peer our blockchain if it is longer
		SendVersion(payload.AddrFrom, chain)
	}

	if NodeIsKnown(payload.AddrFrom) == false {
		KnownNodes = append(KnownNodes, payload.AddrFrom)
	}
}

func HandleTx(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Tx

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	Handle(dec.Decode(&payload))

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
	Handle(dec.Decode(&payload))

	fmt.Printf("Recevied Inventory with %d %s\n", len(payload.Items), payload.Type)

	switch payload.Type {
	case "block":

		// refresh blocksInTransit
		blocksInTransit = payload.Items
		blockHash := payload.Items[0]
		SendGetData(payload.AddrFrom, "block", blockHash)

		newInTransit := [][]byte{}
		for _, b := range blocksInTransit {
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
	Handle(err)

	cmd := Bytes2Cmd(req[:commandLen])
	req = req[commandLen:]
	fmt.Printf("Received %s command\n", cmd)

	//	HexDump([]byte(req))

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
	default:
		fmt.Println("Unknown command")
	}
}

func StartP2PServer(nodeID, minerAddress string) {
	nodeAddr := fmt.Sprintf("localhost:%s", nodeID)
	mineAddress = minerAddress

	ln, err := net.Listen(protocol, nodeAddr)
	Handle(err)
	defer ln.Close()

	chain := blockchain.ContinueBlockChain(nodeID)
	defer chain.Database.Close()
	go CloseDB(chain) // start in goroutine

	if nodeAddr != KnownNodes[0] {
		SendVersion(KnownNodes[0], chain)
	}

	// main server loop
	for {
		fmt.Printf("Waiting for connections at %s\n", nodeAddr)
		conn, err := ln.Accept()
		Handle(err)

		go HandleConnection(conn, chain) // async
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

func HexDump(bytes []byte) {
	var i, j int

	for _, x := range bytes {
		if i%16 == 0 && i != 0 {
			fmt.Printf("|")

			for j = i - 16; j < i; j++ {
				c := rune(bytes[j])
				if strconv.IsPrint(c) {
					fmt.Printf("%c", c)
				} else {
					fmt.Printf(".")
				}
			}
			fmt.Printf("|\n")
		}
		fmt.Printf("%02x ", x)
		i++
	}
	if i != j {
		remaining := j - 16

		for k := i; k < remaining; k++ {
			fmt.Printf("   ")
		}
		fmt.Printf("|")

		for ; j < i; j++ {
			c := rune(bytes[j])
			if strconv.IsPrint(c) {
				fmt.Printf("%c", c)
			} else {
				fmt.Printf(".")
			}
		}
		for k := i; k < remaining; k++ {
			fmt.Printf(" ")
		}
		fmt.Printf("|\n")
	}
}

func Handle(err error) {
	if err != nil {
		log.Panic(err)
	}
}
