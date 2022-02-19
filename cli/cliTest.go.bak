package cli

import (
	"bufio"
	"exx/gochain/blockchain"
	"exx/gochain/network"
	"exx/gochain/wallet"
	"strconv"
	"strings"
	"syscall"

	"fmt"
	"log"
	"os"
	"runtime"

	death "github.com/vrecan/death/v3"
)

type CommandLine struct {
	NodeID     string
	BlockChain *blockchain.BlockChain
}

func (cli *CommandLine) mine(minerAddress string) {
	if !checkAddress(minerAddress) {
		return
	}
}

func (cli *CommandLine) createBlockChain(address string) {
	if !checkAddress(address) {
		return
	}

	UTXOst := blockchain.UTXOSet{
		BlockChain: cli.BlockChain,
	}
	UTXOst.Reindex()
	fmt.Println("Blockchain created")
}

func (cli *CommandLine) getBalance(address string) {
	if !checkAddress(address) {
		return
	}

	UTXOst := blockchain.UTXOSet{
		BlockChain: cli.BlockChain,
	}

	balance := 0
	pubKeyHash := wallet.Base58Decode([]byte(address))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4] // remove version and checksum
	UTXOs := UTXOst.FindUTXO(pubKeyHash)

	for _, out := range UTXOs {
		balance += out.Value
	}

	fmt.Printf("Balance of %s: %d\n", address, balance)
}

func (cli *CommandLine) reindexUTXO() {
	UTXOst := blockchain.UTXOSet{
		BlockChain: cli.BlockChain,
	}
	UTXOst.Reindex()

	count := UTXOst.CountTransactions()
	fmt.Printf("Done! There are %d transactions in the UTXO set.\n", count)
}

func (cli *CommandLine) send(args []string, mineNow bool) {

	// extract args
	from, to := args[0], args[1]
	amount, err := strconv.Atoi(args[2])
	HandleErr(err)

	// check args
	checkAddress(to)
	checkAddress(from)

	UTXOst := blockchain.UTXOSet{
		BlockChain: cli.BlockChain,
	}

	wallets, err := wallet.CreateWallets(cli.NodeID)
	if err != nil {
		log.Panic(err)
	}
	w := wallets.GetWallet(from)

	tx := blockchain.NewTx(&w, to, amount, &UTXOst)
	if mineNow {
		cbTx := blockchain.CoinbaseTx(from, "")
		txs := []*blockchain.Tx{cbTx, tx}
		block := cli.BlockChain.MineBlock(txs)
		UTXOst.Reindex()
		UTXOst.Update(block)
	} else {
		address, err := network.GetAvailablePeer()
		HandleErr(err)
		network.SendTx(address, tx)
		fmt.Printf("Broadcasted transaction to %s\n", address)
	}
	fmt.Printf("Sent %d to %s", amount, to)
}

func (cli *CommandLine) printChain() {
	cli.BlockChain.PrintBlockChain()
}

func (cli *CommandLine) listAddresses() {

	// open wallets
	wallets, err := wallet.CreateWallets(cli.NodeID)
	if os.IsNotExist(err) {
		log.Panic("No wallets")
	}
	addresses := wallets.GetAllAddresses()

	// print wallet addresses
	for _, address := range addresses {
		fmt.Println(address)
	}
}

func (cli *CommandLine) createWallet() {
	wallets, _ := wallet.CreateWallets(cli.NodeID)

	address := wallets.AddWallet()
	wallets.SaveFile(cli.NodeID)

	fmt.Printf("New address is : %s\n", address)
}

func (cli *CommandLine) handleCmd(args []string, nodeID string) {

	// check args
	numArgs := len(args)
	if numArgs < 1 {
		return
	}

	// parse args
	switch args[0] {
	case "getbalance":
		if numArgs < 2 {
			fmt.Fprintf(os.Stderr, "Error: %s <address>\n", args[0])
			return
		}
		cli.getBalance(args[1])
	case "reindexutxo":
		cli.reindexUTXO()
	case "printchain":
		cli.printChain()
	case "listaddresses":
		cli.listAddresses()
	case "createwallet":
		cli.createWallet()
	case "send":
		if numArgs == 4 {
			cli.send(args[1:], false)
		} else if numArgs > 4 {
			cli.send(args[1:], args[4] == "mine")
		} else {
			fmt.Fprintf(os.Stderr,
				"Error: %s from_address to_address amount [mine]\n", args[0])
		}
	default:
		fmt.Fprintf(os.Stderr, "Unrecognised command %s\n", args[0])
	}
}

func (cli *CommandLine) Run() {

	// set NODE_ID
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		fmt.Fprintf(os.Stderr, "NODE_ID environment variable not set\n")
		runtime.Goexit()
	}
	cli.NodeID = nodeID

	// load blockchain
	chain := blockchain.ContinueBlockChain(nodeID)
	defer chain.Database.Close()

	// catch interrupts/signals
	go CloseDB(chain)

	// start node
	go network.StartP2P(chain)

	// read arguments from user input
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		args := strings.Fields(scanner.Text())
		cli.handleCmd(args, nodeID)
	}
}

func HandleErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func checkAddress(address string) bool {
	if !wallet.ValidateAddress(address) {
		fmt.Fprintf(os.Stderr, "Invalid address: %s\n", address)
		return false
	}
	return true
}

func CloseDB(chain *blockchain.BlockChain) {
	die := death.NewDeath(syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	die.WaitForDeathWithFunc(func() {
		defer os.Exit(1)
		defer runtime.Goexit()
		chain.Database.Close()
	})
}
