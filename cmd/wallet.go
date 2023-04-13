package cmd

import (
	// "bufio"
	"fmt"
	"syscall"

	// "os"
	// "strings"

	gethaccounts "github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"github.com/tranvictor/jarvis/accounts"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	TREZOR_BASE_PATH      string = "m/44'/60'/0'/0/%d"
	LEDGER_LIVE_BASE_PATH string = "m/44'/60'/%d'/0/0"
	LEDGER_BASE_PATH      string = "m/44'/60'/0'/%d"

	WALLET_PAGING int = 5
)

var walletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "Manage your wallets",
	Long:  ``,
}

type HW interface {
	Derive(path gethaccounts.DerivationPath) (common.Address, error)
}

func getAccDescsFromHW(hw HW, t string, path string) (*accounts.AccDesc, error) {
	ret := &accounts.AccDesc{
		Kind: t,
	}

	p, err := gethaccounts.ParseDerivationPath(path)
	if err != nil {
		fmt.Printf("Can't parse your %s to get wallets, %s\n", t, err)
		return nil, err
	}

	w, err := hw.Derive(p)
	if err != nil {
		fmt.Printf("Can't read/derive your %s to get wallets, %s. Please check if your ledger is unlocked.\n", t, err)
		return nil, err
	}
	ret.Derpath = p.String()
	ret.Address = w.Hex()
	return ret, nil
}

func handleHW(hw HW, t string) {
	accs := []*accounts.AccDesc{}
	var accDesc *accounts.AccDesc
	var err error

	batch := 0
	for {
		for i := 0; i < WALLET_PAGING; i++ {
			var path string
			switch t {
			case "ledger":
				path = fmt.Sprintf(LEDGER_BASE_PATH, batch*WALLET_PAGING+i)
			case "ledger-live":
				path = fmt.Sprintf(LEDGER_LIVE_BASE_PATH, batch*WALLET_PAGING+i)
			case "trezor":
				path = fmt.Sprintf(TREZOR_BASE_PATH, batch*WALLET_PAGING+i)
			}
			acc, err := getAccDescsFromHW(hw, t, path)
			if err != nil {
				return
			}
			accs = append(accs, acc)
		}
		for i, acc := range accs {
			fmt.Printf("%d. %s (%s)\n", i, acc.Address, acc.Derpath)
		}

		index := cmdutil.PromptIndex("Please enter the wallet index you want to add (0, 1, 2,..., next, back, custom)", 0, len(accs)-1)
		if index == cmdutil.NEXT {
			batch += 1
			continue
		} else if index == cmdutil.BACK {
			if batch > 0 {
				batch -= 1
			} else {
				fmt.Printf("It can't be back. Continue with path 0\n")
			}
			continue
		} else if index == cmdutil.CUSTOM {
			path := cmdutil.PromptInput("Please enter custom derivation path (eg: m/44'/60'/0'/88)")
			accDesc, err = getAccDescsFromHW(hw, t, path)
			if err != nil {
				return
			}
			fmt.Printf("%s (%s)\n", accDesc.Address, accDesc.Derpath)
		} else {
			accDesc = accs[index]
		}

		des := cmdutil.PromptInput("Please enter description of this wallet, it will be used to search your wallet by keywards")
		accDesc.Desc = des
		err := accounts.StoreAccountRecord(*accDesc)
		if err != nil {
			fmt.Printf("Couldn't store your wallet info: %s. Abort.\n", err)
		} else {
			fmt.Printf("Created `~/.jarvis/%s.json` to store the wallet info.\n", accDesc.Address)
			fmt.Printf("Your wallet is added successfully. You can check your list of wallets using the following command:\n> jarvis wallet list\n")
		}
		return
	}
}

func handleLedger(version string) {
	ledger, err := ledgereum.NewLedgereum()
	if err != nil {
		fmt.Printf("Jarvis: Can't establish communication channel to your ledger, %s\n", err)
		return
	}
	err = ledger.Unlock()
	if err != nil {
		fmt.Printf("Jarvis: Can't unlock your ledger, %s\n", err)
		return
	}
	handleHW(ledger, version)
}

func handleTrezor() {
	trezor, err := trezoreum.NewTrezoreum()
	if err != nil {
		fmt.Printf("Jarvis: Can't establish communication channel to your trezor, %s\n", err)
		return
	}
	err = trezor.Unlock()
	if err != nil {
		fmt.Printf("Jarvis: Can't unlock your trezor, %s\n", err)
		return
	}
	handleHW(trezor, "trezor")
}

func handleAddPrivateKey() {
	fmt.Printf("** Storing plain private key is NOT secure. Let's encrypt it to a Keystore.\n")
	fmt.Printf("Please enter or paste your private key in hex format (without 0x prefix). It will not be displayed on your terminal to avoid stdout logging.\n")
	privHex := getPassword("Paste your private key now: ")
	passphrase := getPassword("\nEnter your passcode to encrypt the private key: ")
	fmt.Printf("\n")
	path, err := accounts.StorePrivateKeyWithKeystore(privHex, passphrase)
	if err != nil {
		fmt.Printf("Private key encryption failed: %s. Abort.\n", err)
		return
	}
	fmt.Printf("Stored encrypted private key at %s.\n", path)

	err = handleAddKeystoreGivenPath(path)
	if err != nil {
		fmt.Printf("Adding private key wallet failed: %s\n", err)
		return
	}
}

func getPassword(prompt string) string {
	fmt.Print(prompt)
	bytePassword, _ := terminal.ReadPassword(int(syscall.Stdin))
	return string(bytePassword)
}

func handleAddKeystoreGivenPath(keystorePath string) error {
	accDesc := &accounts.AccDesc{
		Address: "",
		Kind:    "keystore",
		Keypath: keystorePath,
	}
	address, err := accounts.VerifyKeystore(keystorePath)
	if err != nil {
		fmt.Printf("Jarvis: Keystore path verification failed. %s. Abort.\n", err)
		return err
	}
	accDesc.Address = address
	fmt.Printf("Jarvis: This keystore is with %s\n", address)
	des := cmdutil.PromptInput("Jarvis: Please enter description of this wallet, I will look at it to get the wallet for you later based on your search keywords: ")
	accDesc.Desc = des
	err = accounts.StoreAccountRecord(*accDesc)
	if err != nil {
		fmt.Printf("Jarvis: I couldn't store your wallet info: %s. Abort.\n", err)
		return err
	} else {
		fmt.Printf("Jarvis: I created `~/.jarvis/%s.json` to store the keystore info. That file contains the path of your keystore file so please don't move your keystore file later.\n", address)
		fmt.Printf("Jarvis: Your wallet is added successfully. You can check your list of wallets using the following command:\n> jarvis wallet list\n")
	}
	return nil
}

func handleAddKeystore() {
	fmt.Printf("Jarvis: Keystore is convenient but not so safe. I recommend you to use it only for unimportant frequent tasks.\n")
	keystorePath := cmdutil.PromptFilePath("Jarvis: Please enter the path to your keystore file: ")
	handleAddKeystoreGivenPath(keystorePath)
}

var addWalletCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a wallet to jarvis",
	Run: func(cmd *cobra.Command, args []string) {
		// 1. type
		keyType := cmdutil.PromptInput("Jarvis: Enter key type (enter either trezor, ledger, ledger-live, keystore or privatekey):")
		switch keyType {
		case "trezor":
			handleTrezor()
		case "ledger", "ledger-live":
			handleLedger(keyType)
		case "keystore":
			handleAddKeystore()
		case "privatekey":
			handleAddPrivateKey()
		default:
			fmt.Printf("Key: %s is not supported. Abort.\n", keyType)
		}
		// if type is keystore => path to keystore
		// if type is ledger/trezor => show 10 addresses
		// 2. chose address index and register wallet address
	},
}

var listWalletCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all of your wallets",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		accs := accounts.GetAccounts()
		fmt.Printf("Jarvis: You have %d wallets:\n", len(accs))
		index := 0
		for addr, acc := range accs {
			index += 1
			fmt.Printf("%d. %s: %s (%s)\n", index, addr, acc.Kind, acc.Desc)
		}
		fmt.Printf("\nJarvis: If you want to add more wallets to the list, use following command:\n> jarvis wallet add\n")
	},
}

func init() {
	walletCmd.AddCommand(listWalletCmd)
	walletCmd.AddCommand(addWalletCmd)
	rootCmd.AddCommand(walletCmd)
}
