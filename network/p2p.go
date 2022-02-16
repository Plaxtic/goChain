package network

import (
	"bufio"
	"exx/gochain/blockchain"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	death "github.com/vrecan/death/v3" // like signal.h include
)

const (
	aSecond  = 1_000_000_000
	portPath = "./ports"
)

type FileIter struct {
	Line    string
	Scanner *bufio.Scanner
	Fd      *os.File
}

func NewFileIter(filePath string) FileIter {
	file, err := os.Open(filePath)
	HandleErr(err)
	scanner := bufio.NewScanner(file)

	iter := FileIter{
		Line:    scanner.Text(),
		Fd:      file,
		Scanner: scanner,
	}
	return iter
}

func (it *FileIter) Next() bool {

	// increment scanner
	it.Scanner.Scan()

	// update line
	it.Line = it.Scanner.Text()
	if it.Line == "" {
		it.Fd.Close()
		return false
	}
	return true
}

func startServer(chain *blockchain.BlockChain) {
	var address string
	var ln net.Listener
	var err error

	// create port iterator
	ports := NewFileIter(portPath)

	// find avalible port
	for ports.Next() {

		// concat port to host
		address = fmt.Sprintf("localhost:%s", ports.Line)

		// try bind to port
		ln, err = net.Listen(protocol, address)
		if err == nil {
			break
		} else {
			if strings.Contains(err.Error(), "bind: address already in use") {
				continue
			} else {
				log.Panic(err)
			}
		}
	}
	if err != nil {
		log.Fatal("No available ports")
	}
	nodeAddress = address // save our address globaly

	// start server
	fmt.Printf("Sever started on: %s\n", address)
	for {
		conn, err := ln.Accept()
		HandleErr(err)

		HandleConnection(conn, chain)
	}
}

func searchForPeers(chain *blockchain.BlockChain) {

	// create port iterator
	ports := NewFileIter(portPath)

	// iterate ports
	for ports.Next() {

		// concat port to host
		address := fmt.Sprintf("localhost:%s", ports.Line)

		// don't send to self
		if address != nodeAddress && NodeIsKnown(address) == false {

			// try send version
			SendVersion(address, chain)
		}
	}
}

func StartP2P(nodeID, minerAddress string) {

	// save miner address globaly
	mineAddress = minerAddress

	// load blockchain
	chain := blockchain.ContinueBlockChain(nodeID)
	defer chain.Database.Close()

	// catch interrupts/signals
	go CloseDB(chain)

	// start server in go routine
	go startServer(chain)
	time.Sleep(aSecond)

	// scan for peers intermittently
	for {
		fmt.Println("Scanning for peers")
		searchForPeers(chain)

		time.Sleep(30 * aSecond) // 30 seconds
	}
}

func CloseDB(chain *blockchain.BlockChain) {
	die := death.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	die.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		chain.Database.Close()
	})
}
