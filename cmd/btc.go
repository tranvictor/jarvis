package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/tranvictor/jarvis/btc"
	"github.com/tranvictor/jarvis/btc/esplora"
	"github.com/tranvictor/jarvis/btc/signer"
	"github.com/tranvictor/jarvis/btc/spend"
	"github.com/tranvictor/jarvis/btc/wallet"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util/account"
)

var (
	btcChainFlag    string
	btcEsploraFlag  string
	btcPasswordFile string

	btcAddKind     string
	btcAddDesc     string
	btcAddKeystore string
	btcAddKeyFile  string
	btcAddAccount  uint32
	btcAddPath     string
	btcAddForce    bool

	btcFrom             string
	btcTo               string
	btcAmount           string
	btcFeeRate          float64
	btcFeeTarget        int
	btcMaxFeeRate       float64
	btcNoBroadcast      bool
	btcSpendUnconfirmed bool
	btcVerbose          bool
)

const (
	btcDefaultFeeTarget = 2
	btcDefaultMaxRate   = 500.0
	// btcHighFeeShare is the fee-to-payment ratio above which the card warns.
	btcHighFeeShare = 0.10
)

var btcCmd = &cobra.Command{
	Use:   "btc",
	Short: "Bitcoin: wallets, balances, fees and send (native segwit)",
	Long: `Native Bitcoin support. Separate from the EVM commands: it has its own wallets
(keystore, Trezor, Ledger; BIP-84 bc1q addresses), reads coins and fees from an
Esplora API (mempool.space by default) and signs P2WPKH inputs.

  jarvis btc wallet add     add a keystore, Trezor or Ledger wallet
  jarvis btc wallet list    show your Bitcoin wallets
  jarvis btc balance        balance of a wallet or any address
  jarvis btc fees           current network fee rates
  jarvis btc send           pay a bitcoin address from a wallet

Pick the chain with --chain (mainnet, testnet, signet, regtest) when adding a
wallet or reading an address; wallets remember theirs. Point at your own
Esplora with --esplora or JARVIS_BTC_ESPLORA.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var btcWalletCmd = &cobra.Command{
	Use:           "wallet",
	Short:         "Manage your Bitcoin wallets",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func btcChain() (btc.Chain, error) {
	v := btcChainFlag
	if v == "" {
		v = os.Getenv("JARVIS_BTC_CHAIN")
	}
	return btc.ParseChain(v)
}

func btcClient(chain btc.Chain) (*esplora.Client, error) {
	url := btcEsploraFlag
	if url == "" {
		url = os.Getenv("JARVIS_BTC_ESPLORA")
	}
	if url == "" {
		url = chain.DefaultEsplora()
	}
	if url == "" {
		return nil, fmt.Errorf("%s has no public Esplora API: pass --esplora or set JARVIS_BTC_ESPLORA", chain)
	}
	return esplora.New(url), nil
}

func btcContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Minute)
}

func readSecret(prompt, file string) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("no terminal to read a secret from; use --password-file")
	}
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func btcWalletOf(keyword string) (*wallet.Wallet, error) {
	rec, err := wallet.Resolve(keyword)
	if err != nil {
		return nil, err
	}
	return wallet.Open(rec)
}

// ---- wallet add / list ----

var btcWalletAddCmd = &cobra.Command{
	Use:           "add",
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "Add a Bitcoin wallet (keystore, privatekey, trezor or ledger)",
	Long: `Add a Bitcoin wallet.

  keystore     an existing encrypted JSON keystore file; one bc1q address
  privatekey   a private key (hex or WIF) pasted once and stored as an encrypted keystore
  trezor       a Trezor account (m/84'/0'/N'); the device is read once for its public key
  ledger       a Ledger account (m/84'/0'/N'); open the Bitcoin app first

Hardware wallets keep only the account public key on disk, so balances and
fees work with the device unplugged. The device is needed to sign.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		chain, err := btcChain()
		if err != nil {
			return err
		}
		kind := btcAddKind
		if kind == "" {
			kinds := []string{"keystore", "privatekey", "trezor", "ledger"}
			kind = kinds[appUI.Choose("Which kind of Bitcoin wallet?", kinds)]
		}
		desc := btcAddDesc
		if desc == "" {
			desc = strings.TrimSpace(promptLine("Please enter a description — used later to find this wallet by name"))
		}
		name := wallet.Slug(desc)
		if name == "" {
			return errors.New("the description must contain letters or digits")
		}
		if !btcAddForce {
			if _, err := wallet.Resolve(name); err == nil {
				return fmt.Errorf("a bitcoin wallet named %q already exists (use --force to replace it)", name)
			}
		}
		rec := wallet.Record{Name: name, Kind: kind, Chain: string(chain), Desc: desc}

		switch kind {
		case "keystore", "privatekey":
			if err := fillKeystoreRecord(&rec, kind); err != nil {
				return err
			}
		case "trezor", "ledger":
			if err := fillHardwareRecord(&rec, chain); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown wallet kind %q", kind)
		}

		w, err := wallet.Open(rec)
		if err != nil {
			return err
		}
		if err := wallet.Save(rec, btcAddForce); err != nil {
			return err
		}
		first, _ := w.Derive(0, 0)
		dir, _ := wallet.Dir()
		appUI.Success("Added %s wallet %q (%s)", rec.Kind, rec.Name, chain)
		appUI.Info("First address: %s", first.Address)
		appUI.Info("Saved to %s", filepath.Join(dir, rec.Name+".json"))
		if rec.Kind == wallet.KindKeystore {
			appUI.Warn("A keystore is convenient but less safe than a hardware wallet; keep it for small amounts.")
		}
		return nil
	},
}

func promptLine(label string) string {
	appUI.Info("%s", label)
	return appUI.Ask(nil)
}

func fillKeystoreRecord(rec *wallet.Record, kind string) error {
	chain, _ := rec.ChainParsed()
	var path string
	if kind == "privatekey" {
		raw, err := readSecret("Private key (hex or WIF): ", btcAddKeyFile)
		if err != nil {
			return err
		}
		priv, err := parsePrivateKey(raw, chain)
		if err != nil {
			return err
		}
		pass, err := readSecret("New keystore passphrase: ", btcPasswordFile)
		if err != nil {
			return err
		}
		if pass == "" {
			return errors.New("an empty passphrase would leave the key unprotected")
		}
		if btcPasswordFile == "" {
			again, err := readSecret("Repeat passphrase: ", "")
			if err != nil {
				return err
			}
			if again != pass {
				return errors.New("the passphrases differ")
			}
		}
		path, err = writeBTCKeystore(rec.Name, priv, pass)
		if err != nil {
			return err
		}
	} else {
		path = btcAddKeystore
		if path == "" {
			path = strings.TrimSpace(promptLine("Please enter the path to your keystore file"))
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		path = abs
	}
	pass, err := readSecret("Keystore passphrase: ", btcPasswordFile)
	if err != nil {
		return err
	}
	_, ecKey, err := account.PrivateKeyFromKeystore(path, pass)
	if err != nil {
		return fmt.Errorf("unlocking %s: %w", path, err)
	}
	d := make([]byte, 32)
	ecKey.D.FillBytes(d)
	priv, _ := btcec.PrivKeyFromBytes(d)
	rec.Kind = wallet.KindKeystore
	rec.Keystore = path
	rec.PubKey = hex.EncodeToString(priv.PubKey().SerializeCompressed())
	w, err := wallet.Open(*rec)
	if err != nil {
		return err
	}
	first, _ := w.Derive(0, 0)
	rec.Address = first.Address
	return nil
}

func parsePrivateKey(s string, chain btc.Chain) (*btcec.PrivateKey, error) {
	s = strings.TrimSpace(s)
	if len(s) == 64 || (len(s) == 66 && strings.HasPrefix(s, "0x")) {
		raw, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
		if err != nil {
			return nil, errors.New("not a valid hex private key")
		}
		k, _ := btcec.PrivKeyFromBytes(raw)
		return k, nil
	}
	wif, err := btcutil.DecodeWIF(s)
	if err != nil {
		return nil, errors.New("not a hex private key or a WIF")
	}
	if !wif.IsForNet(chain.Params()) {
		return nil, fmt.Errorf("that WIF is for another chain than %s", chain)
	}
	return wif.PrivKey, nil
}

func writeBTCKeystore(name string, priv *btcec.PrivateKey, pass string) (string, error) {
	dir, err := wallet.Dir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "keystores")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	ecKey := priv.ToECDSA()
	key := &gethkeystore.Key{Id: id, PrivateKey: ecKey}
	js, err := gethkeystore.EncryptKey(key, pass, gethkeystore.StandardScryptN, gethkeystore.StandardScryptP)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".json")
	if _, err := os.Stat(path); err == nil && !btcAddForce {
		return "", fmt.Errorf("%s already exists", path)
	}
	return path, os.WriteFile(path, js, 0o600)
}

func fillHardwareRecord(rec *wallet.Record, chain btc.Chain) error {
	acct := chain.AccountPath(btcAddAccount)
	if btcAddPath != "" {
		p, err := btc.ParsePath(btcAddPath)
		if err != nil {
			return err
		}
		if len(p) != 3 || p[0] != 84+0x80000000 || p[1] != chain.CoinType()+0x80000000 || p[2] < 0x80000000 {
			return fmt.Errorf("--path must be a BIP-84 account path m/84'/%d'/N'", chain.CoinType())
		}
		acct = p
	}
	rec.AccountPath = btc.FormatPath(acct)
	var (
		d   *signer.Discovery
		err error
	)
	switch rec.Kind {
	case wallet.KindTrezor:
		appUI.Info("Reading account %s from your Trezor…", rec.AccountPath)
		d, err = signer.DiscoverTrezor(chain, acct, appUI.Info)
	case wallet.KindLedger:
		appUI.Info("Reading account %s from your Ledger. Open the Bitcoin app first…", rec.AccountPath)
		d, err = signer.DiscoverLedger(chain, acct, appUI.Info)
	}
	if err != nil {
		return err
	}
	rec.XPub = d.XPub
	rec.MasterFP = d.MasterFP
	return nil
}

var btcWalletListCmd = &cobra.Command{
	Use:           "list",
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "Show your Bitcoin wallets",
	RunE: func(cmd *cobra.Command, args []string) error {
		recs, err := wallet.List()
		if err != nil {
			return err
		}
		if len(recs) == 0 {
			appUI.Warn("No Bitcoin wallets yet. Add one with: jarvis btc wallet add")
			return nil
		}
		rows := make([][]string, 0, len(recs))
		for _, r := range recs {
			w, err := wallet.Open(r)
			first := "(unreadable record)"
			if err == nil {
				d, _ := w.Derive(0, 0)
				first = d.Address
			}
			acct := r.AccountPath
			if acct == "" {
				acct = "single key"
			}
			rows = append(rows, []string{r.Name, r.Kind, r.Chain, acct, first, r.Desc})
		}
		appUI.Table([]string{"Name", "Kind", "Chain", "Account", "First address", "Description"}, rows)
		return nil
	},
}

// ---- balance ----

var btcBalanceCmd = &cobra.Command{
	Use:   "balance [wallet-or-address]",
	Short: "Show the balance of a Bitcoin wallet or any address",
	Long: `Show confirmed and pending balance. With no argument every wallet is listed.
An address that is not a wallet name is read as a watch-only address on --chain.`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := btcContext()
		defer cancel()
		if len(args) == 0 {
			recs, err := wallet.List()
			if err != nil {
				return err
			}
			if len(recs) == 0 {
				appUI.Warn("No Bitcoin wallets yet. Add one with: jarvis btc wallet add")
				return nil
			}
			var rows [][]string
			for _, r := range recs {
				w, err := wallet.Open(r)
				if err != nil {
					return err
				}
				row, err := walletBalanceRow(ctx, w)
				if err != nil {
					return fmt.Errorf("wallet %s: %w", r.Name, err)
				}
				rows = append(rows, row)
			}
			appUI.Table([]string{"Wallet", "Chain", "Confirmed (BTC)", "Pending (BTC)", "Coins"}, rows)
			return nil
		}

		if rec, err := wallet.Resolve(args[0]); err == nil {
			w, err := wallet.Open(rec)
			if err != nil {
				return err
			}
			client, err := btcClient(w.Chain)
			if err != nil {
				return err
			}
			scan, err := w.Scan(ctx, client)
			if err != nil {
				return err
			}
			printWalletBalance(w, scan)
			return nil
		}

		chain, err := btcChain()
		if err != nil {
			return err
		}
		addr, err := btc.DecodeAddress(args[0], chain.Params())
		if err != nil {
			return fmt.Errorf("%q is neither a bitcoin wallet nor a valid %s address: %w", args[0], chain, err)
		}
		client, err := btcClient(chain)
		if err != nil {
			return err
		}
		st, err := client.Address(ctx, addr.EncodeAddress())
		if err != nil {
			return err
		}
		utxos, err := client.UTXOs(ctx, addr.EncodeAddress())
		if err != nil {
			return err
		}
		appUI.KeyValue([][2]string{
			{"Address", addr.EncodeAddress()},
			{"Chain", string(chain)},
			{"Confirmed", btc.FormatBTC(st.Confirmed) + " BTC"},
			{"Pending", btc.FormatBTC(st.Pending) + " BTC"},
			{"Coins", strconv.Itoa(len(utxos))},
			{"Transactions", strconv.Itoa(st.TxCount)},
		})
		return nil
	},
}

func walletBalanceRow(ctx context.Context, w *wallet.Wallet) ([]string, error) {
	client, err := btcClient(w.Chain)
	if err != nil {
		return nil, err
	}
	scan, err := w.Scan(ctx, client)
	if err != nil {
		return nil, err
	}
	return []string{w.Rec.Name, string(w.Chain), btc.FormatBTC(scan.Confirmed), btc.FormatBTC(scan.Pending), strconv.Itoa(len(scan.Coins))}, nil
}

func printWalletBalance(w *wallet.Wallet, scan *wallet.Scan) {
	rows := [][2]string{
		{"Wallet", fmt.Sprintf("%s (%s, %s)", w.Rec.Name, w.Rec.Kind, w.Chain)},
		{"Confirmed", btc.FormatBTC(scan.Confirmed) + " BTC"},
		{"Pending", btc.FormatBTC(scan.Pending) + " BTC"},
		{"Spendable now", btc.FormatBTC(scan.Spendable()) + " BTC"},
		{"Coins", strconv.Itoa(len(scan.Coins))},
	}
	if w.Rec.IsHD() {
		recv, _ := w.Derive(0, scan.NextReceive)
		rows = append(rows, [2]string{"Next receive address", recv.Address})
	} else {
		d, _ := w.Derive(0, 0)
		rows = append(rows, [2]string{"Address", d.Address})
	}
	appUI.KeyValue(rows)
	if !btcVerbose || len(scan.Coins) == 0 {
		return
	}
	coins := append([]wallet.Coin(nil), scan.Coins...)
	sort.Slice(coins, func(i, j int) bool { return coins[i].Value > coins[j].Value })
	var cr [][]string
	for _, c := range coins {
		state := "pending"
		if c.Confirmed {
			state = "confirmed"
		}
		cr = append(cr, []string{fmt.Sprintf("%s:%d", c.TxID, c.Vout), c.Address, btc.FormatBTC(c.Value), state})
	}
	appUI.Table([]string{"Coin", "Address", "BTC", "State"}, cr)
}

// ---- fees ----

var btcFeesCmd = &cobra.Command{
	Use:           "fees",
	Short:         "Show current Bitcoin network fee rates",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		chain, err := btcChain()
		if err != nil {
			return err
		}
		if len(args) == 0 && btcFrom != "" {
			if rec, err := wallet.Resolve(btcFrom); err == nil {
				chain, _ = rec.ChainParsed()
			}
		}
		client, err := btcClient(chain)
		if err != nil {
			return err
		}
		ctx, cancel := btcContext()
		defer cancel()
		est, err := client.FeeEstimates(ctx)
		if err != nil {
			return err
		}
		if len(est) == 0 {
			return esplora.ErrNoFeeEstimate
		}
		typical := btc.EstimateVSize(1, [][]byte{make([]byte, 22), make([]byte, 22)})
		var rows [][]string
		for _, target := range []int{1, 2, 3, 6, 12, 24, 144} {
			rate, got, err := esplora.RateFor(est, target)
			if err != nil {
				continue
			}
			if got != target {
				continue
			}
			rows = append(rows, []string{
				strconv.Itoa(target), fmt.Sprintf("~%d min", target*10),
				fmt.Sprintf("%.2f", rate), strconv.FormatInt(int64(float64(typical)*rate+0.999), 10),
			})
		}
		appUI.Info("Chain: %s", chain)
		appUI.Table([]string{"Target (blocks)", "Expected wait", "Rate (sat/vB)", fmt.Sprintf("Fee for a %d vB payment (sat)", typical)}, rows)
		if tip, err := client.TipHeight(ctx); err == nil {
			appUI.Info("Chain tip: block %d", tip)
		}
		return nil
	},
}

// ---- send ----

var btcSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send bitcoin from a wallet to an address",
	Long: `Pay a bitcoin address from one of your wallets.

  jarvis btc send --from my-trezor --to bc1q… --amount 0.0025
  jarvis btc send --from my-trezor --to bc1q… --amount ALL --fee-rate 12

--amount takes BTC ("0.001"), satoshis ("150000 sat"), or ALL to sweep every
confirmed coin (the fee comes out of the amount). Without --fee-rate the
network estimate for --target blocks is used. Only confirmed coins are spent
unless --spend-unconfirmed is given. --no-broadcast signs and prints the
transaction hex without sending it.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runBTCSend,
}

func runBTCSend(cmd *cobra.Command, args []string) error {
	ctx, cancel := btcContext()
	defer cancel()

	w, err := btcWalletOf(btcFrom)
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("chain") || os.Getenv("JARVIS_BTC_CHAIN") != "" {
		if c, err := btcChain(); err == nil && c != w.Chain {
			return fmt.Errorf("wallet %q is on %s but --chain says %s", w.Rec.Name, w.Chain, c)
		}
	}
	client, err := btcClient(w.Chain)
	if err != nil {
		return err
	}
	to, err := btc.DecodeAddress(strings.TrimSpace(btcTo), w.Params)
	if err != nil {
		return err
	}

	sweep := strings.EqualFold(strings.TrimSpace(btcAmount), "all")
	var amount int64
	if !sweep {
		if amount, err = btc.ParseAmount(btcAmount); err != nil {
			return err
		}
	}

	rate := btcFeeRate
	source := "--fee-rate"
	if rate == 0 {
		est, err := client.FeeEstimates(ctx)
		if err != nil {
			return fmt.Errorf("reading fee estimates (or pass --fee-rate): %w", err)
		}
		var got int
		rate, got, err = esplora.RateFor(est, btcFeeTarget)
		if err != nil {
			return err
		}
		source = fmt.Sprintf("network estimate for %d blocks", got)
	}
	if rate > btcMaxFeeRate {
		return fmt.Errorf("fee rate %.1f sat/vB is above the %.0f sat/vB safety limit (raise it with --max-fee-rate if you mean it)", rate, btcMaxFeeRate)
	}

	spinner := appUI.Spinner("Scanning wallet…")
	scan, err := w.Scan(ctx, client)
	spinner.Stop(ui.StyledText{Text: "✓ scanned", Severity: ui.SeveritySuccess})
	if err != nil {
		return err
	}
	plan, err := spend.Build(spend.Request{
		Wallet: w, Scan: scan, To: to, Amount: amount, Sweep: sweep,
		FeeRate: rate, SpendUnconfirmed: btcSpendUnconfirmed,
	})
	if err != nil {
		return err
	}

	showPaymentCard(w, plan, to.EncodeAddress(), source)
	if !config.YesToAllPrompt && !appUI.Confirm("Sign this payment?", false) {
		appUI.Warn("Cancelled. Nothing was signed.")
		return nil
	}

	sg, err := btcSignerFor(w, client)
	if err != nil {
		return err
	}
	signed, err := sg.Sign(ctx, plan)
	if err != nil {
		if errors.Is(err, signer.ErrUserCancelled) {
			return fmt.Errorf("%w (nothing was broadcast)", err)
		}
		return err
	}
	if err := plan.Verify(signed); err != nil {
		return fmt.Errorf("refusing to broadcast: the signed transaction failed verification: %w", err)
	}

	var raw bytes.Buffer
	if err := signed.Serialize(&raw); err != nil {
		return err
	}
	rawHex := hex.EncodeToString(raw.Bytes())
	txid := signed.TxHash().String()
	if btcNoBroadcast {
		appUI.Critical("Signed transaction %s (%d vB, not broadcast):", txid, spend.VSize(signed))
		appUI.Critical("%s", rawHex)
		return nil
	}
	got, err := client.Broadcast(ctx, rawHex)
	if err != nil {
		return fmt.Errorf("broadcast failed (the signed transaction is %s): %w", txid, err)
	}
	if got != txid {
		return fmt.Errorf("the server acknowledged transaction %s but we signed %s; check before retrying", got, txid)
	}
	appUI.Success("Broadcast %s", txid)
	return nil
}

func showPaymentCard(w *wallet.Wallet, p *spend.Plan, toAddr, rateSource string) {
	appUI.Section("Confirm Bitcoin payment")
	var payment, change *spend.Output
	for i := range p.Outputs {
		if p.Outputs[i].Change {
			change = &p.Outputs[i]
		} else {
			payment = &p.Outputs[i]
		}
	}
	rows := [][2]string{
		{"Network", string(w.Chain)},
		{"From", fmt.Sprintf("%s (%s)", w.Rec.Name, w.Rec.Kind)},
		{"To", toAddr},
		{"Amount", btc.FormatBTC(payment.Value) + " BTC"},
		{"Fee", fmt.Sprintf("%s BTC (%.2f sat/vB, %s)", btc.FormatBTC(p.Fee), p.FeeRate, rateSource)},
		{"Size", fmt.Sprintf("up to %d vB, %d input(s)", p.VSize, len(p.Inputs))},
	}
	if change != nil {
		note := "change"
		if !w.Rec.IsHD() {
			note = "change, back to the same address"
		}
		rows = append(rows, [2]string{"Change", fmt.Sprintf("%s BTC to %s (%s)", btc.FormatBTC(change.Value), change.Address, note)})
	} else {
		rows = append(rows, [2]string{"Change", "none"})
	}
	rows = append(rows, [2]string{"Total out of wallet", btc.FormatBTC(payment.Value+p.Fee) + " BTC"})
	appUI.KeyValue(rows)
	if payment.Value > 0 && float64(p.Fee) > btcHighFeeShare*float64(payment.Value) {
		appUI.Warn("The fee is %.0f%% of the amount sent.", 100*float64(p.Fee)/float64(payment.Value))
	}
	for _, c := range p.Inputs {
		if !c.Confirmed {
			appUI.Warn("Spending an unconfirmed coin (%s:%d).", c.TxID[:12], c.Vout)
			break
		}
	}
}

func btcSignerFor(w *wallet.Wallet, client *esplora.Client) (signer.Signer, error) {
	switch w.Rec.Kind {
	case wallet.KindKeystore:
		return &signer.Keystore{
			Wallet: w,
			Path:   w.Rec.Keystore,
			Password: func() (string, error) {
				return readSecret(fmt.Sprintf("Passphrase for %s: ", w.Rec.Keystore), btcPasswordFile)
			},
		}, nil
	case wallet.KindTrezor:
		return &signer.Trezor{Wallet: w, Src: client, Notify: appUI.Info}, nil
	case wallet.KindLedger:
		return &signer.Ledger{Wallet: w, Src: client, Notify: appUI.Info}, nil
	}
	return nil, fmt.Errorf("wallet %q has unsupported kind %q", w.Rec.Name, w.Rec.Kind)
}

func init() {
	btcCmd.PersistentFlags().StringVar(&btcChainFlag, "chain", "", "bitcoin chain: mainnet, testnet, signet or regtest (default mainnet; wallets remember theirs)")
	btcCmd.PersistentFlags().StringVar(&btcEsploraFlag, "esplora", "", "Esplora API base URL (default: mempool.space for the chain; env JARVIS_BTC_ESPLORA)")
	btcCmd.PersistentFlags().StringVar(&btcPasswordFile, "password-file", "", "read keystore passphrases from this file instead of asking")

	btcWalletAddCmd.Flags().StringVar(&btcAddKind, "kind", "", "keystore, privatekey, trezor or ledger")
	btcWalletAddCmd.Flags().StringVar(&btcAddDesc, "desc", "", "description; also the wallet's name (lowercased, dashes)")
	btcWalletAddCmd.Flags().StringVar(&btcAddKeystore, "keystore", "", "keystore kind: path to the encrypted JSON keystore")
	btcWalletAddCmd.Flags().StringVar(&btcAddKeyFile, "private-key-file", "", "privatekey kind: read the key from this file instead of asking")
	btcWalletAddCmd.Flags().Uint32Var(&btcAddAccount, "account", 0, "hardware kinds: BIP-44 account number")
	btcWalletAddCmd.Flags().StringVar(&btcAddPath, "path", "", "hardware kinds: explicit account path m/84'/coin'/N'")
	btcWalletAddCmd.Flags().BoolVar(&btcAddForce, "force", false, "replace an existing wallet of the same name")

	btcBalanceCmd.Flags().BoolVarP(&btcVerbose, "verbose", "v", false, "also list every coin")

	btcFeesCmd.Flags().StringVar(&btcFrom, "from", "", "use the chain of this wallet")

	btcSendCmd.Flags().StringVar(&btcFrom, "from", "", "wallet to pay from (name, description or address)")
	btcSendCmd.Flags().StringVarP(&btcTo, "to", "t", "", "bitcoin address to pay")
	btcSendCmd.Flags().StringVarP(&btcAmount, "amount", "v", "", `amount: "0.001" (BTC), "150000 sat", or ALL`)
	btcSendCmd.Flags().Float64Var(&btcFeeRate, "fee-rate", 0, "fee rate in sat/vB (default: network estimate)")
	btcSendCmd.Flags().IntVar(&btcFeeTarget, "target", btcDefaultFeeTarget, "confirmation target in blocks for the fee estimate")
	btcSendCmd.Flags().Float64Var(&btcMaxFeeRate, "max-fee-rate", btcDefaultMaxRate, "refuse fee rates above this many sat/vB")
	btcSendCmd.Flags().BoolVar(&btcNoBroadcast, "no-broadcast", false, "sign and print the transaction hex, do not send it")
	btcSendCmd.Flags().BoolVar(&btcSpendUnconfirmed, "spend-unconfirmed", false, "also spend coins that are not confirmed yet")
	_ = btcSendCmd.MarkFlagRequired("from")
	_ = btcSendCmd.MarkFlagRequired("to")
	_ = btcSendCmd.MarkFlagRequired("amount")

	btcWalletCmd.AddCommand(btcWalletAddCmd)
	btcWalletCmd.AddCommand(btcWalletListCmd)
	btcCmd.AddCommand(btcWalletCmd)
	btcCmd.AddCommand(btcBalanceCmd)
	btcCmd.AddCommand(btcFeesCmd)
	btcCmd.AddCommand(btcSendCmd)
	rootCmd.AddCommand(btcCmd)
}
