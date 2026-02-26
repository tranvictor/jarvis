package cmd

import (
	"fmt"
	"sort"
	"syscall"

	gethaccounts "github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
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

func getAccDescsFromHW(hw HW, t string, path string) (*types.AccDesc, error) {
	ret := &types.AccDesc{
		Kind: t,
	}

	p, err := gethaccounts.ParseDerivationPath(path)
	if err != nil {
		appUI.Error("Can't parse your %s to get wallets, %s", t, err)
		return nil, err
	}

	w, err := hw.Derive(p)
	if err != nil {
		appUI.Error("Can't read/derive your %s to get wallets, %s. Please check if your ledger is unlocked.", t, err)
		return nil, err
	}
	ret.Derpath = p.String()
	ret.Address = w.Hex()
	return ret, nil
}

func handleHW(hw HW, t string) {
	var accs []*types.AccDesc
	var accDesc *types.AccDesc
	var err error

	batch := 0
	for {
		for i := 0; i < WALLET_PAGING; i++ {
			var pathTemplate string
			switch t {
			case "ledger":
				pathTemplate = LEDGER_BASE_PATH
			case "ledger-live":
				pathTemplate = LEDGER_LIVE_BASE_PATH
			case "trezor":
				pathTemplate = TREZOR_BASE_PATH
			}
			path := fmt.Sprintf(pathTemplate, batch*WALLET_PAGING+i)
			acc, err := getAccDescsFromHW(hw, t, path)
			if err != nil {
				return
			}
			accs = append(accs, acc)
		}
		for i, acc := range accs {
			appUI.Info("%d. %s (%s)", i, acc.Address, acc.Derpath)
		}

		index := cmdutil.PromptIndex(appUI, "Please enter the wallet index you want to add (0, 1, 2,..., next, back, custom)", 0, len(accs)-1)
		if index == cmdutil.NEXT {
			batch += 1
			continue
		} else if index == cmdutil.BACK {
			if batch > 0 {
				batch -= 1
			} else {
				appUI.Warn("It can't be back. Continue with path 0")
			}
			continue
		} else if index == cmdutil.CUSTOM {
			path := cmdutil.PromptInput(appUI, "Please enter custom derivation path (eg: m/44'/60'/0'/88)")
			accDesc, err = getAccDescsFromHW(hw, t, path)
			if err != nil {
				return
			}
			appUI.Info("%s (%s)", accDesc.Address, accDesc.Derpath)
		} else {
			accDesc = accs[index]
		}

		des := cmdutil.PromptInput(appUI, "Please enter description of this wallet, it will be used to search your wallet by keywords")
		accDesc.Desc = des
		if err = accounts.StoreAccountRecord(*accDesc); err != nil {
			appUI.Error("Couldn't store your wallet info: %s. Abort.", err)
		} else {
			appUI.Success("Created ~/.jarvis/%s.json to store the wallet info.", accDesc.Address)
			appUI.Info("Your wallet is added successfully. You can check your list of wallets using the following command:\n> jarvis wallet list")
		}
		return
	}
}

func handleLedger(version string) {
	ledger, err := ledgereum.NewLedgereum()
	if err != nil {
		appUI.Error("Can't establish communication channel to your ledger, %s", err)
		return
	}
	err = ledger.Unlock()
	if err != nil {
		appUI.Error("Can't unlock your ledger, %s", err)
		return
	}
	handleHW(ledger, version)
}

func handleTrezor() {
	trezor, err := trezoreum.NewTrezoreum()
	if err != nil {
		appUI.Error("Can't establish communication channel to your trezor, %s", err)
		return
	}
	err = trezor.Unlock()
	if err != nil {
		appUI.Error("Can't unlock your trezor, %s", err)
		return
	}
	handleHW(trezor, "trezor")
}

func handleAddPrivateKey() {
	appUI.Warn("Storing plain private key is NOT secure. Let's encrypt it to a Keystore.")
	appUI.Info("Please enter or paste your private key in hex format (without 0x prefix). It will not be displayed on your terminal to avoid stdout logging.")
	privHex := getPassword("Paste your private key now: ")
	passphrase := getPassword("\nEnter your passcode to encrypt the private key: ")
	appUI.Info("")
	path, err := accounts.StorePrivateKeyWithKeystore(privHex, passphrase)
	if err != nil {
		appUI.Error("Private key encryption failed: %s. Abort.", err)
		return
	}
	appUI.Success("Stored encrypted private key at %s.", path)

	if err = handleAddKeystoreGivenPath(path); err != nil {
		appUI.Error("Adding private key wallet failed: %s", err)
	}
}

func getPassword(prompt string) string {
	appUI.Info(prompt)
	bytePassword, _ := terminal.ReadPassword(int(syscall.Stdin))
	return string(bytePassword)
}

func handleAddKeystoreGivenPath(keystorePath string) error {
	accDesc := &types.AccDesc{
		Address: "",
		Kind:    "keystore",
		Keypath: keystorePath,
	}
	address, err := accounts.VerifyKeystore(keystorePath)
	if err != nil {
		appUI.Error("Keystore path verification failed. %s. Abort.", err)
		return err
	}
	accDesc.Address = address
	appUI.Info("This keystore is with %s", address)
	des := cmdutil.PromptInput(appUI, "Please enter description of this wallet, I will look at it to get the wallet for you later based on your search keywords")
	accDesc.Desc = des
	if err = accounts.StoreAccountRecord(*accDesc); err != nil {
		appUI.Error("I couldn't store your wallet info: %s. Abort.", err)
		return err
	}
	appUI.Success("I created ~/.jarvis/%s.json to store the keystore info. That file contains the path of your keystore file so please don't move your keystore file later.", address)
	appUI.Info("Your wallet is added successfully. You can check your list of wallets using the following command:\n> jarvis wallet list")
	return nil
}

func handleAddKeystore() {
	appUI.Warn("Keystore is convenient but not so safe. I recommend you to use it only for unimportant frequent tasks.")
	keystorePath := cmdutil.PromptFilePath(appUI, "Please enter the path to your keystore file")
	handleAddKeystoreGivenPath(keystorePath)
}

var addWalletCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a wallet to jarvis",
	Run: func(cmd *cobra.Command, args []string) {
		keyType := cmdutil.PromptInput(appUI, "Enter key type (enter either trezor, ledger, ledger-live, keystore or privatekey):")
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
			appUI.Error("Key: %s is not supported. Abort.", keyType)
		}
	},
}

var listWalletCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all of your wallets",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		accs := accounts.GetAccounts()
		appUI.Info("You have %d wallets:", len(accs))

		type accountInfo struct {
			addr string
			acc  types.AccDesc
		}
		var accountList []accountInfo
		for addr, acc := range accs {
			accountList = append(accountList, accountInfo{addr: addr, acc: acc})
		}
		sort.Slice(accountList, func(i, j int) bool {
			return accountList[i].acc.Desc < accountList[j].acc.Desc
		})
		for index, item := range accountList {
			appUI.Info("%d. %s: %s (%s)", index+1, item.addr, item.acc.Kind, item.acc.Desc)
		}
		appUI.Info("\nIf you want to add more wallets to the list, use following command:\n> jarvis wallet add")
	},
}

func init() {
	walletCmd.AddCommand(listWalletCmd)
	walletCmd.AddCommand(addWalletCmd)
	rootCmd.AddCommand(walletCmd)
}
