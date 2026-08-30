package trezoreum

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/golang/protobuf/proto"

	"github.com/tranvictor/jarvis/util/account/trezoreum/trezor"
	"github.com/tranvictor/jarvis/util/account/usb"
)

const (
	TrezorScheme         string = "trezor"
	VendorIDWithHID      uint16 = 0x534c
	VendorIDWithWebUSB   uint16 = 0x1209
	UsageIDWIthHID       uint16 = 0xff00
	UsageIDWIthWebUSB    uint16 = 0xffff
	EndPointIDWithHID    int    = 0x0
	EndPointIDWithWebUSB int    = 0x0
)

var (
	ProductIDsWithHID    []uint16 = []uint16{0x0001}
	ProductIDsWithWebUSB []uint16 = []uint16{0x53c1}
)

type Trezoreum struct {
	core         *TrezorDriver
	devmu        sync.Mutex
	features     *trezor.Features
	expectedAddr common.Address
}

func NewTrezoreum() (*Trezoreum, error) {
	return &Trezoreum{
		core:  NewTrezorDriver(),
		devmu: sync.Mutex{},
	}, nil
}

func (self *Trezoreum) SetExpectedAddress(addr common.Address) {
	self.expectedAddr = addr
}

func (self *Trezoreum) Unlock() error {
	self.devmu.Lock()
	defer self.devmu.Unlock()
	info, state, err := self.Init()
	if err != nil {
		return err
	}
	fmt.Printf(
		"Firmware version: %d.%d.%d\n",
		info.GetMajorVersion(),
		info.GetMinorVersion(),
		info.GetPatchVersion(),
	)
	for state != Ready {
		if state == WaitingForPin {
			pin := PromptPINFromStdin()
			_, err = self.UnlockByPin(pin)
			if err != nil {
				fmt.Printf("Pin error: %s\n", err)
				continue
			}
			state = Ready
		} else if state == WaitingForPassphrase {
			_, err = self.PromptAndProvidePassphrase()
			if err != nil {
				fmt.Printf("Passphrase error: %s\n", err)
				continue
			}
			state = Ready
		} else {
			return fmt.Errorf("unexpected trezor state")
		}
	}
	return nil
}

// trezorExchange performs a data exchange with the Trezor wallet, sending it a
// message and retrieving the response. If multiple responses are possible, the
// method will also return the index of the destination object used.
func (self *Trezoreum) trezorExchange(req proto.Message, results ...proto.Message) (int, error) {
	results = append(results, new(trezor.PinMatrixRequest))
	results = append(results, new(trezor.PassphraseRequest))
	resIndex, err := self.core.Exchange(req, results...)
	if err != nil {
		return resIndex, err
	}

	// if response is a matrix request
	if resIndex == len(results)-2 {
		pin := PromptPINFromStdin()
		resIndex, err = self.UnlockByPin(pin, results...)
		if err != nil {
			fmt.Printf("Pin error 2: %s\n", err)
			return resIndex, err
		}
		return resIndex, nil
	}

	// if response is a passphrse request
	if resIndex == len(results)-1 {
		return self.PromptAndProvidePassphrase(results...)
	}
	return resIndex, err
}

func (self *Trezoreum) PromptAndProvidePassphrase(results ...proto.Message) (int, error) {
	return self.promptAndProvidePassphrase(false, results...)
}

func (self *Trezoreum) promptAndProvidePassphrase(skipCache bool, results ...proto.Message) (int, error) {
	if !skipCache {
		if cached, ok := lookupCachedPassphrase(self.expectedAddr); ok {
			if cached.onDevice {
				fmt.Printf("Reusing on-device passphrase from this session\n")
				return self.ProvidePassphrase("", true, results...)
			}
			fmt.Printf("Reusing passphrase %s\n", passphraseMask)
			return self.ProvidePassphrase(cached.secret, false, results...)
		}
	}

	if self.passphraseAlwaysOnDevice() {
		fmt.Printf("Enter the passphrase on the Trezor\n")
		return self.ProvidePassphrase("", true, results...)
	}

	passphrase := promptPassphraseFromStdin()
	resIndex, err := self.ProvidePassphrase(passphrase, false, results...)
	if err != nil {
		fmt.Printf("Passphrase error: %s\n", err)
		return resIndex, err
	}
	return resIndex, nil
}

func (self *Trezoreum) passphraseAlwaysOnDevice() bool {
	return self.features != nil && self.features.GetPassphraseAlwaysOnDevice()
}

func (self *Trezoreum) rememberFeatures(f *trezor.Features) {
	if f == nil {
		return
	}
	self.features = f
	if sid := f.GetSessionId(); len(sid) > 0 {
		storeSessionID(sid)
	}
}

// trezorTransport describes one USB identity a Trezor can present. Trezor
// Model T / Safe expose a WebUSB interface (vendor 0x1209), while the Trezor
// One and bridge-fronted devices expose a legacy HID interface (vendor
// 0x534c). We probe both so detection doesn't depend on the model / firmware /
// whether Trezor Suite or Bridge has reshaped the device.
type trezorTransport struct {
	vendor     uint16
	productIDs []uint16
	usageID    uint16
	endpointID int
}

func (self *Trezoreum) GetDevice() ([]usb.DeviceInfo, error) {
	// WebUSB is listed first so that when both identities are present the
	// WebUSB device is preferred (devices[0]), preserving prior behavior for
	// Model T / Safe users.
	transports := []trezorTransport{
		{
			vendor:     VendorIDWithWebUSB,
			productIDs: ProductIDsWithWebUSB,
			usageID:    UsageIDWIthWebUSB,
			endpointID: EndPointIDWithWebUSB,
		},
		{
			vendor:     VendorIDWithHID,
			productIDs: ProductIDsWithHID,
			usageID:    UsageIDWIthHID,
			endpointID: EndPointIDWithHID,
		},
	}

	devices := []usb.DeviceInfo{}
	var firstErr error

	for _, t := range transports {
		infos, err := usb.Enumerate(t.vendor, 0)
		if err != nil {
			// Remember the first enumeration error but keep probing the other
			// transport; one failing must not mask a device on the other.
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		for _, info := range infos {
			for _, id := range t.productIDs {
				// Windows and Macos use UsageID matching, Linux uses Interface matching
				if info.ProductID == id && (info.UsagePage == t.usageID || info.Interface == t.endpointID) {
					devices = append(devices, info)
					break
				}
			}
		}
	}

	// Only surface an enumeration error when it actually prevented us from
	// finding anything; if one transport erred but the other yielded a device
	// we proceed with what we have.
	if len(devices) == 0 && firstErr != nil {
		return devices, firstErr
	}
	return devices, nil
}

func (self *Trezoreum) Init() (*trezor.Features, TrezorState, error) {
	devices, err := self.GetDevice()
	if err != nil {
		return nil, Unexpected, err
	}
	if len(devices) == 0 {
		return nil, Unexpected, fmt.Errorf("Couldn't find any trezor devices")
	}

	// assume we only have 1 valid device
	device := devices[0]
	driver, err := device.Open()
	if err != nil {
		return nil, Unexpected, fmt.Errorf("Couldn't open trezor device: %s", err)
	}
	self.core.SetDevice(driver)

	features := &trezor.Features{}
	success := trezor.Success{}
	initMsg := trezor.Initialize{SessionId: currentSessionID()}
	_, err = self.trezorExchange(&initMsg, features, &success)
	if err != nil && len(initMsg.SessionId) > 0 {
		forgetDeviceSession()
		features = &trezor.Features{}
		_, err = self.trezorExchange(&trezor.Initialize{}, features, &success)
	}
	if err != nil {
		return nil, Unexpected, err
	}
	self.rememberFeatures(features)

	res, err := self.trezorExchange(
		&trezor.Ping{},
		new(trezor.PinMatrixRequest),
		new(trezor.PassphraseRequest),
		new(trezor.Success),
		features,
	)
	if err != nil {
		return nil, Unexpected, err
	}

	self.rememberFeatures(features)
	switch res {
	case 0:
		return features, WaitingForPin, nil
	case 1:
		return features, WaitingForPassphrase, nil
	case 2:
		return features, Ready, nil
	case 3:
		return features, Ready, nil
	default:
		return features, Ready, nil
	}
}

func (self *Trezoreum) UnlockByPin(pin string, results ...proto.Message) (int, error) {
	// res, err := self.trezorExchange(&trezor.PinMatrixAck{Pin: &pin}, new(trezor.Success), new(trezor.PassphraseRequest))
	results = append(results, new(trezor.Success))
	results = append(results, new(trezor.PassphraseRequest))
	// fmt.Printf("DEBUG trezor comms: PinMatrixAck message, expecting passphrase, success message\n")
	res, err := self.core.Exchange(
		&trezor.PinMatrixAck{Pin: &pin},
		results...,
	)
	if err != nil {
		return 0, err
	}
	if res == len(results)-1 {
		return self.PromptAndProvidePassphrase(results...)
	}
	return res, nil
}

func (self *Trezoreum) ProvidePassphrase(passphrase string, onDevice bool, results ...proto.Message) (int, error) {
	ack := &trezor.PassphraseAck{}
	if onDevice {
		yes := true
		ack.OnDevice = &yes
	} else {
		ack.Passphrase = &passphrase
	}
	results = append(results, new(trezor.Success))
	results = append(results, new(trezor.PassphraseRequest))
	res, err := self.core.Exchange(ack, results...)
	if err != nil {
		return 0, err
	}

	if res == len(results)-1 {
		return self.promptAndProvidePassphrase(true, results...)
	}
	rememberPending(passphrase, onDevice)
	return res, nil
}

func (self *Trezoreum) Derive(path accounts.DerivationPath) (common.Address, error) {
	address := new(trezor.EthereumAddress)
	// fmt.Printf("DEBUG trezor comms: getAddress message, expecting EthereumAddress message\n")
	if _, err := self.trezorExchange(&trezor.EthereumGetAddress{AddressN: path}, address); err != nil {
		return common.Address{}, err
	}
	if addr := address.GetAddress(); len(addr) > 0 { // Older firmwares use binary fomats
		return common.HexToAddress(addr), nil
	}
	return common.Address{}, errors.New("missing derived address")
}

func (self *Trezoreum) SignDynamicFeeTx(
	path accounts.DerivationPath,
	tx *types.Transaction,
	chainId *big.Int,
) (common.Address, *types.Transaction, error) {
	// Create the transaction initiation message
	data := tx.Data()
	length := uint32(len(data))

	request := &trezor.EthereumSignTxEIP1559{
		AddressN:       path,
		Nonce:          new(big.Int).SetUint64(tx.Nonce()).Bytes(),
		MaxGasFee:      tx.GasFeeCap().Bytes(),
		MaxPriorityFee: tx.GasTipCap().Bytes(),
		GasLimit:       new(big.Int).SetUint64(tx.Gas()).Bytes(),
		Value:          tx.Value().Bytes(),
		DataLength:     &length,
	}
	if to := tx.To(); to != nil {
		// Non contract deploy, set recipient explicitly
		hex := to.Hex()
		request.To = &hex // Newer firmwares (old will ignore)
		// request.ToBin = (*to)[:] // Older firmwares (new will ignore)
	}
	if length > 1024 { // Send the data chunked if that was requested
		request.DataInitialChunk, data = data[:1024], data[1024:]
	} else {
		request.DataInitialChunk, data = data, nil
	}
	if chainId != nil { // EIP-155 transaction, set chain ID explicitly (only 32 bit is supported!?)
		id := chainId.Uint64()
		request.ChainId = &id
	}
	// Send the initiation message and stream content until a signature is returned
	response := new(trezor.EthereumTxRequest)
	// fmt.Printf(
	// 	"DEBUG trezor comms: EthereumSignTxEIP1559 message, expecting EthereumTxRequest message\n",
	// )
	if _, err := self.trezorExchange(request, response); err != nil {
		return common.Address{}, nil, err
	}
	for response.DataLength != nil && int(*response.DataLength) <= len(data) {
		chunk := data[:*response.DataLength]
		data = data[*response.DataLength:]

		// fmt.Printf(
		// 	"DEBUG trezor comms: EthereumTxAck message, expecting EthereumTxRequest message\n",
		// )
		if _, err := self.trezorExchange(&trezor.EthereumTxAck{DataChunk: chunk}, response); err != nil {
			return common.Address{}, nil, err
		}
	}

	// Extract the Ethereum signature and do a sanity validation
	if len(response.GetSignatureR()) == 0 || len(response.GetSignatureS()) == 0 {
		return common.Address{}, nil, errors.New("reply lacks signature")
	}
	signature := append(
		append(response.GetSignatureR(), response.GetSignatureS()...),
		byte(response.GetSignatureV()),
	)

	// Create the correct signer and signature transform based on the chain ID
	var signer types.Signer
	if chainId == nil {
		signer = new(types.HomesteadSigner)
	} else {
		signer = types.LatestSignerForChainID(chainId)
		// signature[64] -= byte(chainId.Uint64()*2 + 35)
	}

	// Inject the final signature into the transaction and sanity check the sender
	signed, err := tx.WithSignature(signer, signature)
	if err != nil {
		return common.Address{}, nil, err
	}
	sender, err := types.Sender(signer, signed)
	if err != nil {
		return common.Address{}, nil, err
	}
	return sender, signed, nil
}

func (self *Trezoreum) SignLegacyTx(
	path accounts.DerivationPath,
	tx *types.Transaction,
	chainId *big.Int,
) (common.Address, *types.Transaction, error) {
	// Create the transaction initiation message
	data := tx.Data()
	length := uint32(len(data))

	request := &trezor.EthereumSignTx{
		AddressN:   path,
		Nonce:      new(big.Int).SetUint64(tx.Nonce()).Bytes(),
		GasPrice:   tx.GasPrice().Bytes(),
		GasLimit:   new(big.Int).SetUint64(tx.Gas()).Bytes(),
		Value:      tx.Value().Bytes(),
		DataLength: &length,
	}
	if to := tx.To(); to != nil {
		// Non contract deploy, set recipient explicitly
		hex := to.Hex()
		request.To = &hex // Newer firmwares (old will ignore)
		// request.ToBin = (*to)[:] // Older firmwares (new will ignore)
	}
	if length > 1024 { // Send the data chunked if that was requested
		request.DataInitialChunk, data = data[:1024], data[1024:]
	} else {
		request.DataInitialChunk, data = data, nil
	}
	if chainId != nil { // EIP-155 transaction, set chain ID explicitly (only 32 bit is supported!?)
		id := chainId.Uint64()
		request.ChainId = &id
	}
	// Send the initiation message and stream content until a signature is returned
	response := new(trezor.EthereumTxRequest)
	// fmt.Printf("DEBUG trezor comms: EthereumSignTx message, expecting EthereumTxRequest message\n")
	if _, err := self.trezorExchange(request, response); err != nil {
		return common.Address{}, nil, err
	}
	for response.DataLength != nil && int(*response.DataLength) <= len(data) {
		chunk := data[:*response.DataLength]
		data = data[*response.DataLength:]

		// fmt.Printf(
		// 	"DEBUG trezor comms: EthereumTxAck message, expecting EthereumTxRequest message\n",
		// )
		if _, err := self.trezorExchange(&trezor.EthereumTxAck{DataChunk: chunk}, response); err != nil {
			return common.Address{}, nil, err
		}
	}
	// Extract the Ethereum signature and do a sanity validation
	if len(response.GetSignatureR()) == 0 || len(response.GetSignatureS()) == 0 ||
		response.GetSignatureV() == 0 {
		return common.Address{}, nil, errors.New("reply lacks signature")
	}
	signature := append(
		append(response.GetSignatureR(), response.GetSignatureS()...),
		byte(response.GetSignatureV()),
	)

	// Create the correct signer and signature transform based on the chain ID
	var signer types.Signer
	if chainId == nil {
		signer = new(types.HomesteadSigner)
	} else {
		signer = types.LatestSignerForChainID(chainId)
		signature[64] -= byte(chainId.Uint64()*2 + 35)
	}

	// Inject the final signature into the transaction and sanity check the sender
	signed, err := tx.WithSignature(signer, signature)
	if err != nil {
		return common.Address{}, nil, err
	}
	sender, err := types.Sender(signer, signed)
	if err != nil {
		return common.Address{}, nil, err
	}
	return sender, signed, nil
}

func (self *Trezoreum) Sign(
	path accounts.DerivationPath,
	tx *types.Transaction,
	chainId *big.Int,
) (common.Address, *types.Transaction, error) {
	if tx.Type() == types.LegacyTxType {
		return self.SignLegacyTx(path, tx, chainId)
	} else if tx.Type() == types.DynamicFeeTxType {
		return self.SignDynamicFeeTx(path, tx, chainId)
	}

	return common.Address{}, nil, fmt.Errorf("not supported type - trezoreum can't sign")
}

// SignTypedHash uses EthereumSignTypedHash (added in firmware 2.4.3 / 1.10.5)
// to sign a precomputed EIP-712 digest. The device shows the two raw hashes
// to the user — blind-signing must be enabled.
func (self *Trezoreum) SignTypedHash(
	path accounts.DerivationPath,
	domainSeparator [32]byte,
	messageHash [32]byte,
) ([]byte, error) {
	req := &trezor.EthereumSignTypedHash{
		AddressN:            path,
		DomainSeparatorHash: domainSeparator[:],
		MessageHash:         messageHash[:],
	}
	resp := new(trezor.EthereumTypedDataSignature)
	if _, err := self.trezorExchange(req, resp); err != nil {
		return nil, err
	}
	if len(resp.Signature) != 65 {
		return nil, fmt.Errorf(
			"trezor EthereumTypedDataSignature has length %d, want 65",
			len(resp.Signature),
		)
	}
	sig := make([]byte, 65)
	copy(sig, resp.Signature)
	// Trezor returns v in {0, 1} for some firmware revisions; normalise to
	// the canonical {27, 28} pair used by the Safe contract.
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig, nil
}

// SignPersonalMessage signs an arbitrary byte string with the EIP-191
// "personal_sign" prefix. Used as the fallback path when the connected
// Trezor firmware is too old for EthereumSignTypedHash.
func (self *Trezoreum) SignPersonalMessage(
	path accounts.DerivationPath,
	message []byte,
) ([]byte, error) {
	msg := make([]byte, len(message))
	copy(msg, message)
	req := &trezor.EthereumSignMessage{
		AddressN: path,
		Message:  msg,
	}
	resp := new(trezor.EthereumMessageSignature)
	if _, err := self.trezorExchange(req, resp); err != nil {
		return nil, err
	}
	if len(resp.Signature) != 65 {
		return nil, fmt.Errorf(
			"trezor EthereumMessageSignature has length %d, want 65",
			len(resp.Signature),
		)
	}
	sig := make([]byte, 65)
	copy(sig, resp.Signature)
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig, nil
}
