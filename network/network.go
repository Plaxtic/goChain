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
	protocol     = "tcp"
	version      = 1
	commandLen   = 12
	maxTXPoolSiz = 2
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

type GetBlock struct {
	AddrFrom string
	Idx      int
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
	BestHeight int64 // to compare blockchain lengths
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

func SendTx(address string, tx *blockchain.Tx) {
	data := Tx{
		AddrFrom:    nodeAddress,
		Transaction: tx.ToBytes(),
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

	fmt.Printf("Syncing blocks, %d remaining\n", len(blocksInTransit))

	if len(blocksInTransit) > 0 {

		blockHash := blocksInTransit[0]
		SendGetData(payload.AddrFrom, "block", blockHash)

		blocksInTransit = blocksInTransit[1:]
	} else {
		fmt.Println("\nSynced")
	}
	UTXOst := blockchain.UTXOSet{
		BlockChain: chain,
	}
	UTXOst.Reindex()
}
func HandleGetBlock(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload GetBlock

	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

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
		fmt.Printf("New peer at: %s\n", payload.AddrFrom)
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
		fmt.Printf("New peer at: %s\n", payload.AddrFrom)
		KnownNodes = append(KnownNodes, payload.AddrFrom)
	}
}

func HandleTx(request []byte, chain *blockchain.BlockChain) {
	var buff bytes.Buffer
	var payload Tx

	// decode
	buff.Write(request)
	dec := gob.NewDecoder(&buff)
	HandleErr(dec.Decode(&payload))

	// get transaction from payload
	txData := payload.Transaction
	tx := blockchain.Bytes2Tx(txData)

	// check not in chain yet
	_, err := chain.FindTx(tx.ID)
	if err == nil {
		return
	}

	// add to pool
	memoryPool[hex.EncodeToString(tx.ID)] = tx

	// mine block if transation pool full and mining on
	/*
		mine := len(mineAddress) > 0 && len(memoryPool) >= maxTXPoolSiz
		if mine {
			MineTx(chain)
		}
	*/
	for _, node := range KnownNodes {
		if node != nodeAddress && node != payload.AddrFrom {

			/*
				// broadcast transaction or new chain
				if !mine {
					SendInv(node, "tx", [][]byte{tx.ID})
				} else { */
			SendVersion(node, chain)
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

		// if no blockchain, get all
		if _, err := chain.GetLastBlock(); err != nil {
			blocksInTransit = payload.Items
		} else {

			// get only new blocks
			for i, h := range payload.Items {
				if chain.ContainsBlock(h) {
					blocksInTransit = payload.Items[:i]
					break
				}
				if i == len(payload.Items)-1 {
					blocksInTransit = payload.Items
				}
			}
		}
		if len(blocksInTransit) > 0 {
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
		}

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

	// skip bad connections
	if len(req) < commandLen {
		return
	}

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

func EmptyPool(chain *blockchain.BlockChain) (txs []*blockchain.Tx) {
	for id := range memoryPool {
		tx := memoryPool[id]
		delete(memoryPool, id)

		if chain.VerifyTx(&tx) {
			txs = append(txs, &tx)
		}
	}
	return txs
}

func MineTx(chain *blockchain.BlockChain) {

	// mine new block
	fmt.Println("Mining...")
	newBlock := chain.MineBlock([]*blockchain.Tx{})

	// add transactions
	txs := EmptyPool(chain)
	cbTx := blockchain.CoinbaseTx(mineAddress, "")
	txs = append(txs, cbTx)
	newBlock.Txs = txs

	// add block to chain
	chain.AddBlock(newBlock)

	// refresh UTXOs
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
