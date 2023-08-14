package trezoreum

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/usbwallet/trezor"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/golang/protobuf/proto"
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
	// session string
	core  *TrezorDriver
	devmu sync.Mutex
}

func NewTrezoreum() (*Trezoreum, error) {
	return &Trezoreum{
		core:  NewTrezorDriver(),
		devmu: sync.Mutex{},
	}, nil
}

func (self *Trezoreum) Unlock() error {
	self.devmu.Lock()
	defer self.devmu.Unlock()
	info, state, err := self.Init()
	if err != nil {
		return err
	}
	fmt.Printf("Firmware version: %d.%d.%d\n", *info.MajorVersion, *info.MinorVersion, *info.PatchVersion)
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
			return fmt.Errorf("Not support passphrase yet")
		}
	}
	return nil
}

// trezorExchange performs a data exchange with the Trezor wallet, sending it a
// message and retrieving the response. If multiple responses are possible, the
// method will also return the index of the destination object used.
func (self *Trezoreum) trezorExchange(req proto.Message, results ...proto.Message) (int, error) {
	results = append(results, new(trezor.PinMatrixRequest))
	resIndex, err := self.core.Exchange(req, results...)
	if err != nil {
		return resIndex, err
	}
	if resIndex == len(results)-1 {
		pin := PromptPINFromStdin()
		resIndex, err = self.UnlockByPin(pin, results...)
		if err != nil {
			fmt.Printf("Pin error 2: %s\n", err)
			return resIndex, err
		}
		return resIndex, nil
	}
	return resIndex, err
}

func (self *Trezoreum) GetDevice() ([]usb.DeviceInfo, error) {
	// vendor := VendorIDWithHID
	// productIDs := ProductIDsWithHID
	// usageID := UsageIDWIthHID
	// endpointID := EndPointIDWithHID

	vendor := VendorIDWithWebUSB
	productIDs := ProductIDsWithWebUSB
	usageID := UsageIDWIthWebUSB
	endpointID := EndPointIDWithWebUSB

	devices := []usb.DeviceInfo{}

	infos, err := usb.Enumerate(vendor, 0)
	if err != nil {
		return devices, err
	}

	for _, info := range infos {
		for _, id := range productIDs {
			// Windows and Macos use UsageID matching, Linux uses Interface matching
			if info.ProductID == id && (info.UsagePage == usageID || info.Interface == endpointID) {
				devices = append(devices, info)
				break
			}
		}
	}
	return devices, nil
}

func (self *Trezoreum) Init() (trezor.Features, TrezorState, error) {
	devices, err := self.GetDevice()

	if err != nil {
		return trezor.Features{}, Unexpected, err
	}
	if len(devices) == 0 {
		return trezor.Features{}, Unexpected, fmt.Errorf("Couldn't find any trezor devices")
	}

	// assume we only have 1 valid device
	device := devices[0]
	driver, err := device.Open()
	if err != nil {
		return trezor.Features{}, Unexpected, fmt.Errorf("Couldn't open trezor device: %s", err)
	}
	self.core.SetDevice(driver)
	// session := device.Session
	// if session == nil {
	// 	self.session = ""
	// } else {
	// 	self.session = *session
	// }
	// s, err := self.core.Acquire(device.Path, self.session, false)
	// if err != nil {
	// 	return trezor.Features{}, Unexpected, err
	// }
	// self.session = s

	// test init device
	initMsg := trezor.Initialize{}
	features := trezor.Features{}

	_, err = self.trezorExchange(&initMsg, &features)
	if err != nil {
		return trezor.Features{}, Unexpected, err
	}

	askPin := true
	askPassphrase := true
	res, err := self.trezorExchange(
		&trezor.Ping{PinProtection: &askPin, PassphraseProtection: &askPassphrase},
		new(trezor.PinMatrixRequest),
		new(trezor.PassphraseRequest),
		new(trezor.Success),
	)
	if err != nil {
		return trezor.Features{}, Unexpected, err
	}

	switch res {
	case 0:
		return features, WaitingForPin, nil
	case 1:
		return features, WaitingForPassphrase, nil
	case 2:
		// if *features.PinCached {
		// 	return features, Ready, nil
		// } else {
		// 	return features, WaitingForPin, nil
		// }
		return features, Ready, nil
	default:
		return features, Ready, nil
	}
}

func (self *Trezoreum) UnlockByPin(pin string, results ...proto.Message) (int, error) {
	// res, err := self.trezorExchange(&trezor.PinMatrixAck{Pin: &pin}, new(trezor.Success), new(trezor.PassphraseRequest))
	results = append(results, new(trezor.Success))
	results = append(results, new(trezor.PassphraseRequest))
	res, err := self.core.Exchange(
		&trezor.PinMatrixAck{Pin: &pin},
		results...,
	)
	if err != nil {
		return 0, err
	}
	if res == len(results)-1 {
		// this is to handle passphrase
		return 0, fmt.Errorf("passphrase is not supported")
	}
	return res, nil
}

func (self *Trezoreum) UnlockByPassphrase(passphrase string) (TrezorState, error) {
	return Unexpected, fmt.Errorf("Not implemented")
}

func (self *Trezoreum) Derive(path accounts.DerivationPath) (common.Address, error) {
	address := new(trezor.EthereumAddress)
	if _, err := self.trezorExchange(&trezor.EthereumGetAddress{AddressN: path}, address); err != nil {
		return common.Address{}, err
	}
	if addr := address.GetAddressBin(); len(addr) > 0 { // Older firmwares use binary fomats
		return common.BytesToAddress(addr), nil
	}
	if addr := address.GetAddressHex(); len(addr) > 0 { // Newer firmwares use hexadecimal fomats
		return common.HexToAddress(addr), nil
	}
	return common.Address{}, errors.New("missing derived address")
}

func (self *Trezoreum) Sign(path accounts.DerivationPath, tx *types.Transaction, chainID *big.Int) (common.Address, *types.Transaction, error) {
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
		// fmt.Printf("hex: %s\n", hex)
		request.ToHex = &hex     // Newer firmwares (old will ignore)
		request.ToBin = (*to)[:] // Older firmwares (new will ignore)
	}
	if length > 1024 { // Send the data chunked if that was requested
		request.DataInitialChunk, data = data[:1024], data[1024:]
	} else {
		request.DataInitialChunk, data = data, nil
	}
	if chainID != nil { // EIP-155 transaction, set chain ID explicitly (only 32 bit is supported!?)
		id := uint32(chainID.Int64())
		request.ChainId = &id
	}
	// Send the initiation message and stream content until a signature is returned
	response := new(trezor.EthereumTxRequest)
	if _, err := self.trezorExchange(request, response); err != nil {
		return common.Address{}, nil, err
	}
	for response.DataLength != nil && int(*response.DataLength) <= len(data) {
		chunk := data[:*response.DataLength]
		data = data[*response.DataLength:]

		if _, err := self.trezorExchange(&trezor.EthereumTxAck{DataChunk: chunk}, response); err != nil {
			return common.Address{}, nil, err
		}
	}
	// Extract the Ethereum signature and do a sanity validation
	if len(response.GetSignatureR()) == 0 || len(response.GetSignatureS()) == 0 || response.GetSignatureV() == 0 {
		return common.Address{}, nil, errors.New("reply lacks signature")
	}
	signature := append(append(response.GetSignatureR(), response.GetSignatureS()...), byte(response.GetSignatureV()))

	// Create the correct signer and signature transform based on the chain ID
	var signer types.Signer
	if chainID == nil {
		signer = new(types.HomesteadSigner)
	} else {
		signer = types.NewEIP155Signer(chainID)
		signature[64] -= byte(chainID.Uint64()*2 + 35)
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
