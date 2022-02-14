package cli

import (
	"exx/gochain/blockchain"
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
	fmt.Println(" send -from FROM -to TO -amount AMOUNT - Send amount of coins")
	fmt.Println(" createwallet - Creates a new Wallet")
	fmt.Println(" listaddresses - Lists the addresses in our wallet file")
	fmt.Println(" reindexutxo - Rebuilds the UTXO set")
}

func (cli *CommandLine) validateArgs() {
	if len(os.Args) < 2 {
		cli.printUsage()
		runtime.Goexit() // exits application by only closing Go routine to allow clean exit of database
	}
}

func (cli *CommandLine) createBlockChain(address string) {
	checkAddress(address)

	chain := blockchain.InitBlockChain(address)
	chain.Database.Close()

	UTXOst := blockchain.UTXOSet{BlockChain: chain}
	UTXOst.Reindex()
	fmt.Println("Blockchain created")
}

func (cli *CommandLine) getBalance(address string) {
	checkAddress(address)

	chain := blockchain.ContinueBlockChain(address)
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

func (cli *CommandLine) reindexUTXO() {
	chain := blockchain.ContinueBlockChain("")
	defer chain.Database.Close()
	UTXOSet := blockchain.UTXOSet{BlockChain: chain}
	UTXOSet.Reindex()

	count := UTXOSet.CountTransactions()
	fmt.Printf("Done! There are %d transactions in the UTXO set.\n", count)
}

func (cli *CommandLine) send(from, to string, amount int) {
	checkAddress(to)
	checkAddress(from)

	chain := blockchain.ContinueBlockChain(from)
	UTXOst := blockchain.UTXOSet{BlockChain: chain}
	defer chain.Database.Close()

	tx := blockchain.NewTx(from, to, amount, &UTXOst)
	cbTx := blockchain.CoinbaseTx(from, "")
	block := chain.AddBlock([]*blockchain.Tx{cbTx, tx})
	UTXOst.Update(block)
	fmt.Printf("Sent %d to %s", amount, to)
}

func (cli *CommandLine) printChain() {
	chain := blockchain.ContinueBlockChain("")
	defer chain.Database.Close()
	chain.PrintBlockChain()
}

func (cli *CommandLine) listAddresses() {
	wallets, err := wallet.CreateWallets()
	if os.IsNotExist(err) {
		log.Panic("No wallets")
	}

	addresses := wallets.GetAllAddresses()

	for _, address := range addresses {
		fmt.Println(address)
	}
}

func (cli *CommandLine) createWallet() {
	wallets, _ := wallet.CreateWallets()

	address := wallets.AddWallet()
	wallets.SaveFile()

	fmt.Printf("New address is : %s\n", address)
}

func (cli *CommandLine) Run() {
	cli.validateArgs()

	// get options
	getBalanceCmd := flag.NewFlagSet("getbalance", flag.ExitOnError)
	createBlockchainCmd := flag.NewFlagSet("createblockchain", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("printchain", flag.ExitOnError)
	createWalletCmd := flag.NewFlagSet("createwallet", flag.ExitOnError)
	listAddressesCmd := flag.NewFlagSet("listaddresses", flag.ExitOnError)
	reindexUTXOCmd := flag.NewFlagSet("reindexutxo", flag.ExitOnError)

	// get optargs
	getBalanceAddress := getBalanceCmd.String("address", "", "The address to get balance for")
	createBlockchainAddress := createBlockchainCmd.String("address", "", "The address to send genesis block reward to")
	sendFrom := sendCmd.String("from", "", "Source wallet address")
	sendTo := sendCmd.String("to", "", "Destination wallet address")
	sendAmount := sendCmd.Int("amount", 0, "Amount to send")

	// handle options
	switch os.Args[1] {
	case "getbalance":
		err := getBalanceCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "reindexutxo":
		err := reindexUTXOCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "createblockchain":
		err := createBlockchainCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "printchain":
		err := printChainCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "listaddresses":
		err := listAddressesCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "createwallet":
		err := createWalletCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	case "send":
		err := sendCmd.Parse(os.Args[2:])
		if err != nil {
			log.Panic(err)
		}
	default:
		cli.printUsage()
		runtime.Goexit()
	}
	if getBalanceCmd.Parsed() {
		if *getBalanceAddress == "" {
			getBalanceCmd.Usage()
			runtime.Goexit()
		}
		cli.getBalance(*getBalanceAddress)
	}
	if createBlockchainCmd.Parsed() {
		if *createBlockchainAddress == "" {
			createBlockchainCmd.Usage()
			runtime.Goexit()
		}
		cli.createBlockChain(*createBlockchainAddress)
	}
	if reindexUTXOCmd.Parsed() {
		cli.reindexUTXO()
	}
	if printChainCmd.Parsed() {
		cli.printChain()
	}
	if createWalletCmd.Parsed() {
		cli.createWallet()
	}
	if listAddressesCmd.Parsed() {
		cli.listAddresses()
	}
	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount <= 0 {
			sendCmd.Usage()
			runtime.Goexit()
		}
		cli.send(*sendFrom, *sendTo, *sendAmount)
	}
}

func checkAddress(address string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("Invalid address")
	}
}
