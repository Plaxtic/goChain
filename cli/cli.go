package cli

import (
	"exx/gochain/blockchain"
	"exx/gochain/network"
	"exx/gochain/wallet"

	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
)

type CommandLine struct{}

func (cli *CommandLine) printUsage() {
	fmt.Println("Usage:")
	fmt.Println(" getbalance -address ADDRESS - get the balance for an address")
	fmt.Println(" createblockchain -address ADDRESS creates a blockchain and sends genesis reward to address")
	fmt.Println(" printchain - Prints the blocks in the chain")
	fmt.Println(" send -from FROM -to TO -amount AMOUNT -mine - Send amount of coins. Then -mine flag is set, mine off of this node")
	fmt.Println(" createwallet - Creates a new Wallet")
	fmt.Println(" listaddresses - Lists the addresses in our wallet file")
	fmt.Println(" reindexutxo - Rebuilds the UTXO set")
	fmt.Println(" startnode -miner ADDRESS - Start a node with ID specified in NODE_ID env. var. -miner enables mining")
}

func (cli *CommandLine) validateArgs() {
	if len(os.Args) < 2 {
		cli.printUsage()
		runtime.Goexit() // exits application by only closing Go routine to allow clean exit of database
	}
}

func (cli *CommandLine) startNode(nodeID, minerAddress string) {
	fmt.Printf("Starting node %s\n", nodeID)

	if len(minerAddress) > 0 {
		if wallet.ValidateAddress(minerAddress) {
			fmt.Println("Mining is on. Address to receive rewards ", minerAddress)
		} else {
			log.Panic("Invalid address")
		}
	}
	network.StartP2P(nodeID, minerAddress)
}

func (cli *CommandLine) createBlockChain(address, nodeID string) {
	checkAddress(address)

	chain := blockchain.InitBlockChain(address, nodeID)
	chain.Database.Close()

	UTXOst := blockchain.UTXOSet{
		BlockChain: chain,
	}
	UTXOst.Reindex()
	fmt.Println("Blockchain created")
}

func (cli *CommandLine) getBalance(address, nodeID string) {
	checkAddress(address)

	chain := blockchain.ContinueBlockChain(nodeID)
	UTXOst := blockchain.UTXOSet{BlockChain: chain}
	defer chain.Database.Close()

	balance := 0
	pubKeyHash := wallet.Base58Decode([]byte(address))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4] // remove version and checksum
	UTXOs := UTXOst.FindUTXO(pubKeyHash)

	for _, out := range UTXOs {
		balance += out.Value
	}

	fmt.Printf("Balance of %s: %d\n", address, balance)
}

func (cli *CommandLine) reindexUTXO(nodeID string) {
	chain := blockchain.ContinueBlockChain(nodeID)
	defer chain.Database.Close()
	UTXOst := blockchain.UTXOSet{
		BlockChain: chain,
	}
	UTXOst.Reindex()

	count := UTXOst.CountTransactions()
	fmt.Printf("Done! There are %d transactions in the UTXO set.\n", count)
}

func (cli *CommandLine) printChain(nodeID string) {
	chain := blockchain.ContinueBlockChain(nodeID)
	defer chain.Database.Close()
	chain.PrintBlockChain()
}

func (cli *CommandLine) listAddresses(nodeID string) {
	wallets, err := wallet.CreateWallets(nodeID)
	if os.IsNotExist(err) {
		log.Panic("No wallets")
	}

	addresses := wallets.GetAllAddresses()

	for _, address := range addresses {
		fmt.Println(address)
	}
}

func (cli *CommandLine) createWallet(nodeID string) {
	wallets, _ := wallet.CreateWallets(nodeID)

	address := wallets.AddWallet()
	wallets.SaveFile(nodeID)

	fmt.Printf("New address is : %s\n", address)
}

func (cli *CommandLine) send(from, to string, amount int, nodeID string, mineNow bool) {
	checkAddress(to)
	checkAddress(from)

	chain := blockchain.ContinueBlockChain(nodeID)
	UTXOst := blockchain.UTXOSet{
		BlockChain: chain,
	}
	defer chain.Database.Close()

	wallets, err := wallet.CreateWallets(nodeID)
	HandleErr(err)

	w := wallets.GetWallet(from)
	tx := blockchain.NewTx(&w, to, amount, &UTXOst)

	if mineNow {
		cbTx := blockchain.CoinbaseTx(from, "")
		txs := []*blockchain.Tx{cbTx, tx}
		block := chain.MineBlock(txs)
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
	cli.validateArgs()

	// check for NODE_ID environment variable
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		fmt.Fprintf(os.Stderr, "NODE_ID environment variable not set\n")
		runtime.Goexit()
	}

	// get options
	getBalanceCmd := flag.NewFlagSet("getbalance", flag.ExitOnError)
	createBlockchainCmd := flag.NewFlagSet("createblockchain", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("printchain", flag.ExitOnError)
	createWalletCmd := flag.NewFlagSet("createwallet", flag.ExitOnError)
	listAddressesCmd := flag.NewFlagSet("listaddresses", flag.ExitOnError)
	reindexUTXOCmd := flag.NewFlagSet("reindexutxo", flag.ExitOnError)
	startNodeCmd := flag.NewFlagSet("startnode", flag.ExitOnError)

	// get optargs
	getBalanceAddress := getBalanceCmd.String("address", "", "The address to get balance for")
	createBlockchainAddress := createBlockchainCmd.String("address", "", "The address to send genesis block reward to")
	sendFrom := sendCmd.String("from", "", "Source wallet address")
	sendTo := sendCmd.String("to", "", "Destination wallet address")
	sendAmount := sendCmd.Int("amount", 0, "Amount to send")
	sendMine := sendCmd.Bool("mine", false, "Mine immediately on the same node")
	startNodeMiner := startNodeCmd.String("miner", "", "Enable mining mode and send reward to ADDRESS")

	// handle options
	switch os.Args[1] {
	case "getbalance":
		HandleErr(getBalanceCmd.Parse(os.Args[2:]))
	case "reindexutxo":
		HandleErr(reindexUTXOCmd.Parse(os.Args[2:]))
	case "createblockchain":
		HandleErr(createBlockchainCmd.Parse(os.Args[2:]))
	case "printchain":
		HandleErr(printChainCmd.Parse(os.Args[2:]))
	case "listaddresses":
		HandleErr(listAddressesCmd.Parse(os.Args[2:]))
	case "startnode":
		HandleErr(startNodeCmd.Parse(os.Args[2:]))
	case "createwallet":
		HandleErr(createWalletCmd.Parse(os.Args[2:]))
	case "send":
		HandleErr(sendCmd.Parse(os.Args[2:]))
	default:
		cli.printUsage()
		runtime.Goexit()
	}
	if getBalanceCmd.Parsed() {
		if *getBalanceAddress == "" {
			getBalanceCmd.Usage()
			runtime.Goexit()
		}
		cli.getBalance(*getBalanceAddress, nodeID)
	}
	if createBlockchainCmd.Parsed() {
		if *createBlockchainAddress == "" {
			createBlockchainCmd.Usage()
			runtime.Goexit()
		}
		cli.createBlockChain(*createBlockchainAddress, nodeID)
	}
	if reindexUTXOCmd.Parsed() {
		cli.reindexUTXO(nodeID)
	}
	if printChainCmd.Parsed() {
		cli.printChain(nodeID)
	}
	if createWalletCmd.Parsed() {
		cli.createWallet(nodeID)
	}
	if listAddressesCmd.Parsed() {
		cli.listAddresses(nodeID)
	}
	if startNodeCmd.Parsed() {
		cli.startNode(nodeID, *startNodeMiner)
	}
	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount <= 0 {
			sendCmd.Usage()
			runtime.Goexit()
		}
		cli.send(*sendFrom, *sendTo, *sendAmount, nodeID, *sendMine)
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
