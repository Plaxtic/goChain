package cli

import (
	"exx/gochain/blockchain"
	"exx/gochain/network"
	"exx/gochain/wallet"
	"strconv"
	"syscall"

	"fmt"
	"log"
	"os"
	"runtime"

	death "github.com/vrecan/death/v3"
)

type CommandLine struct {
	BlockChain *blockchain.BlockChain
	nodeID     string
}

func (cli *CommandLine) printUsage() {
	fmt.Println("Usage:")
	fmt.Println("	--balance ADDRESS            - get the balance for an address")
	fmt.Println("	--createblockchain ADDRESS   - creates a blockchain and sends genesis reward to address")
	fmt.Println("	--print                      - Prints the blocks in the chain")
	fmt.Println("	--send FROM TO AMOUNT [mine] - Send amount of coins. Then -mine flag is set, mine off of this node")
	fmt.Println("	--createwallet               - Creates a new Wallet")
	fmt.Println("	--listaddresses              - Lists the addresses in our wallet file")
	fmt.Println("	--reindexutxo                - Rebuilds the UTXO set")
	fmt.Println("	--startnode [ADDRESS]        - Start a node with ID specified in NODE_ID env. var. -miner enables mining")
}

func (cli *CommandLine) startNode(minerAddress string) {
	fmt.Printf("Starting node %s\n", cli.nodeID)

	if len(minerAddress) > 0 {
		if wallet.ValidateAddress(minerAddress) {
			fmt.Println("Mining is on. Address to receive rewards ", minerAddress)
		} else {
			log.Panic("Invalid address")
		}
	}
	network.StartP2P(cli.BlockChain, minerAddress)
}

func (cli *CommandLine) getBalance(address string) {
	checkAddress(address)

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

func (cli *CommandLine) printChain() {
	cli.BlockChain.PrintBlockChain()
}

func (cli *CommandLine) listAddresses() {
	wallets, err := wallet.CreateWallets(cli.nodeID)
	if os.IsNotExist(err) {
		log.Panic("No wallets")
	}

	addresses := wallets.GetAllAddresses()

	for _, address := range addresses {
		fmt.Println(address)
	}
}

func (cli *CommandLine) createWallet() string {
	wallets, _ := wallet.CreateWallets(cli.nodeID)

	address := wallets.AddWallet()
	wallets.SaveFile(cli.nodeID)

	fmt.Printf("New address is : %s\n", address)
	return address
}

func (cli *CommandLine) send(from, to, num string, mineNow bool) {
	checkAddress(to)
	checkAddress(from)

	amount, err := strconv.Atoi(num)
	HandleErr(err)

	UTXOst := blockchain.UTXOSet{
		BlockChain: cli.BlockChain,
	}

	wallets, err := wallet.CreateWallets(cli.nodeID)
	HandleErr(err)

	w := wallets.GetWallet(from)
	tx := blockchain.NewTx(&w, to, amount, &UTXOst)

	if mineNow {
		cbTx := blockchain.CoinbaseTx(from, "")
		txs := []*blockchain.Tx{cbTx, tx}
		block := cli.BlockChain.MineBlock(txs)
		UTXOst.Update(block)
	} else {
		address, err := network.GetAvailablePeer()
		HandleErr(err)
		network.SendTx(address, tx)
		fmt.Printf("Broadcasted transaction to %s\n", address)
	}
	fmt.Printf("Sent %d to %s\n", amount, to)
}

func (cli *CommandLine) Run() {

	// get NODE_ID environment variable
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		fmt.Fprintf(os.Stderr, "NODE_ID environment variable not set\n")
		runtime.Goexit()
	}
	cli.nodeID = nodeID

	// create blockchain if none
	path := fmt.Sprintf(blockchain.DBPath, nodeID)
	if !blockchain.DBexists(path) {
		newWalletAddress := cli.createWallet()
		cli.BlockChain = blockchain.InitBlockChain(newWalletAddress, nodeID)
	} else {
		cli.BlockChain = blockchain.ContinueBlockChain(nodeID)
	}

	// close database safely
	defer cli.BlockChain.Database.Close()
	go CloseDB(cli.BlockChain)

	// default start node
	if len(os.Args) == 1 {
		cli.startNode("")
	}

	// handle options
	switch os.Args[1] {

	case "--reindexutxo":
		cli.reindexUTXO()
	case "--print":
		cli.printChain()
	case "--listaddresses":
		cli.listAddresses()
	case "--createwallet":
		cli.createWallet()
	case "--balance":
		if len(os.Args) < 3 {
			cli.printUsage()
			runtime.Goexit()
		}
		cli.getBalance(os.Args[2])
	case "--startnode":
		if len(os.Args) > 3 {
			cli.startNode(os.Args[3])
		} else {
			cli.startNode("")
		}
	case "--send":
		if len(os.Args) < 5 {
			cli.printUsage()
			runtime.Goexit()
		} else if len(os.Args) < 6 {
			cli.send(os.Args[2], os.Args[3], os.Args[4], false)
		}
	default:
		cli.printUsage()
		runtime.Goexit()
	}
}

func HandleErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func checkAddress(address string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("Invalid address")
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
