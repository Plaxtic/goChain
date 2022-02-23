package network

import (
	"bufio"
	"errors"
	"exx/gochain/blockchain"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
	// like signal.h include
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
	if !it.Scanner.Scan() {
		it.Fd.Close()
		return false
	}

	// update line
	it.Line = it.Scanner.Text()
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

func GetAvailablePeer() (address string, err error) {

	// create port iterator
	ports := NewFileIter(portPath)

	// iterate ports
	for ports.Next() {

		// concat port to host
		address = fmt.Sprintf("localhost:%s", ports.Line)

		// try connect
		conn, err := net.Dial(protocol, address)
		if err == nil {
			conn.Close()
			return address, err
		}
	}
	return "", errors.New("No peers avalible")
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

func Mine(chain *blockchain.BlockChain) {
	for {
		MineTx(chain)
	}
}

func StartP2P(chain *blockchain.BlockChain, minerAddress string) {

	// set miner address globaly
	mineAddress = minerAddress

	// start server in go routine
	go startServer(chain)
	time.Sleep(aSecond)

	// start miner
	if mineAddress != "" {
		go Mine(chain)
	}

	// scan for peers intermittently
	for {
		fmt.Println("Scanning for peers")
		searchForPeers(chain)

		time.Sleep(30 * aSecond) // 30 seconds
	}
}
