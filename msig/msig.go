package msig

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	. "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/reader"
)

const GNOSIS_MULTISIG_ABI string = `[{"constant":true,"inputs":[{"name":"","type":"uint256"}],"name":"owners","outputs":[{"name":"","type":"address"}],"payable":false,"type":"function"},{"constant":false,"inputs":[{"name":"owner","type":"address"}],"name":"removeOwner","outputs":[],"payable":false,"type":"function"},{"constant":false,"inputs":[{"name":"transactionId","type":"uint256"}],"name":"revokeConfirmation","outputs":[],"payable":false,"type":"function"},{"constant":true,"inputs":[{"name":"","type":"address"}],"name":"isOwner","outputs":[{"name":"","type":"bool"}],"payable":false,"type":"function"},{"constant":true,"inputs":[{"name":"","type":"uint256"},{"name":"","type":"address"}],"name":"confirmations","outputs":[{"name":"","type":"bool"}],"payable":false,"type":"function"},{"constant":true,"inputs":[],"name":"calcMaxWithdraw","outputs":[{"name":"","type":"uint256"}],"payable":false,"type":"function"},{"constant":true,"inputs":[{"name":"pending","type":"bool"},{"name":"executed","type":"bool"}],"name":"getTransactionCount","outputs":[{"name":"count","type":"uint256"}],"payable":false,"type":"function"},{"constant":true,"inputs":[],"name":"dailyLimit","outputs":[{"name":"","type":"uint256"}],"payable":false,"type":"function"},{"constant":true,"inputs":[],"name":"lastDay","outputs":[{"name":"","type":"uint256"}],"payable":false,"type":"function"},{"constant":false,"inputs":[{"name":"owner","type":"address"}],"name":"addOwner","outputs":[],"payable":false,"type":"function"},{"constant":true,"inputs":[{"name":"transactionId","type":"uint256"}],"name":"isConfirmed","outputs":[{"name":"","type":"bool"}],"payable":false,"type":"function"},{"constant":true,"inputs":[{"name":"transactionId","type":"uint256"}],"name":"getConfirmationCount","outputs":[{"name":"count","type":"uint256"}],"payable":false,"type":"function"},{"constant":true,"inputs":[{"name":"","type":"uint256"}],"name":"transactions","outputs":[{"name":"destination","type":"address"},{"name":"value","type":"uint256"},{"name":"data","type":"bytes"},{"name":"executed","type":"bool"}],"payable":false,"type":"function"},{"constant":true,"inputs":[],"name":"getOwners","outputs":[{"name":"","type":"address[]"}],"payable":false,"type":"function"},{"constant":true,"inputs":[{"name":"from","type":"uint256"},{"name":"to","type":"uint256"},{"name":"pending","type":"bool"},{"name":"executed","type":"bool"}],"name":"getTransactionIds","outputs":[{"name":"_transactionIds","type":"uint256[]"}],"payable":false,"type":"function"},{"constant":true,"inputs":[{"name":"transactionId","type":"uint256"}],"name":"getConfirmations","outputs":[{"name":"_confirmations","type":"address[]"}],"payable":false,"type":"function"},{"constant":true,"inputs":[],"name":"transactionCount","outputs":[{"name":"","type":"uint256"}],"payable":false,"type":"function"},{"constant":false,"inputs":[{"name":"_required","type":"uint256"}],"name":"changeRequirement","outputs":[],"payable":false,"type":"function"},{"constant":false,"inputs":[{"name":"transactionId","type":"uint256"}],"name":"confirmTransaction","outputs":[],"payable":false,"type":"function"},{"constant":false,"inputs":[{"name":"destination","type":"address"},{"name":"value","type":"uint256"},{"name":"data","type":"bytes"}],"name":"submitTransaction","outputs":[{"name":"transactionId","type":"uint256"}],"payable":false,"type":"function"},{"constant":false,"inputs":[{"name":"_dailyLimit","type":"uint256"}],"name":"changeDailyLimit","outputs":[],"payable":false,"type":"function"},{"constant":true,"inputs":[],"name":"MAX_OWNER_COUNT","outputs":[{"name":"","type":"uint256"}],"payable":false,"type":"function"},{"constant":true,"inputs":[],"name":"required","outputs":[{"name":"","type":"uint256"}],"payable":false,"type":"function"},{"constant":false,"inputs":[{"name":"owner","type":"address"},{"name":"newOwner","type":"address"}],"name":"replaceOwner","outputs":[],"payable":false,"type":"function"},{"constant":false,"inputs":[{"name":"transactionId","type":"uint256"}],"name":"executeTransaction","outputs":[],"payable":false,"type":"function"},{"constant":true,"inputs":[],"name":"spentToday","outputs":[{"name":"","type":"uint256"}],"payable":false,"type":"function"},{"inputs":[{"name":"_owners","type":"address[]"},{"name":"_required","type":"uint256"},{"name":"_dailyLimit","type":"uint256"}],"type":"constructor"},{"payable":true,"type":"fallback"},{"anonymous":false,"inputs":[{"indexed":false,"name":"dailyLimit","type":"uint256"}],"name":"DailyLimitChange","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"sender","type":"address"},{"indexed":true,"name":"transactionId","type":"uint256"}],"name":"Confirmation","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"sender","type":"address"},{"indexed":true,"name":"transactionId","type":"uint256"}],"name":"Revocation","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"transactionId","type":"uint256"}],"name":"Submission","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"transactionId","type":"uint256"}],"name":"Execution","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"transactionId","type":"uint256"}],"name":"ExecutionFailure","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"sender","type":"address"},{"indexed":false,"name":"value","type":"uint256"}],"name":"Deposit","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"owner","type":"address"}],"name":"OwnerAddition","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"owner","type":"address"}],"name":"OwnerRemoval","type":"event"},{"anonymous":false,"inputs":[{"indexed":false,"name":"required","type":"uint256"}],"name":"RequirementChange","type":"event"}]`

type MultisigContract struct {
	Address string
	Network Network
	reader  *reader.EthReader
	Abi     *abi.ABI
}

func (self *MultisigContract) Owners() ([]string, error) {
	owners := new([]common.Address)
	err := self.reader.ReadContractWithABI(
		owners,
		self.Address,
		self.Abi,
		"getOwners",
	)
	result := []string{}
	if err != nil {
		return result, err
	}
	for _, owner := range *owners {
		result = append(result, owner.Hex())
	}
	return result, nil
}

func (self *MultisigContract) IsExecuted(txid *big.Int) (bool, error) {
	_, _, _, executed, _, err := self.TransactionInfo(txid)
	return executed, err
}

func (self *MultisigContract) IsConfirmed(txid *big.Int) (bool, error) {
	r := new(bool)
	err := self.reader.ReadContractWithABI(
		r,
		self.Address,
		self.Abi,
		"isConfirmed",
		txid,
	)
	if err != nil {
		return false, err
	}
	return *r, err
}

func (self *MultisigContract) NOTransactions() (int64, error) {
	r := big.NewInt(0)
	err := self.reader.ReadContractWithABI(
		&r,
		self.Address,
		self.Abi,
		"transactionCount",
	)
	if err != nil {
		return 0, err
	}
	return r.Int64(), nil
}

func (self *MultisigContract) VoteRequirement() (int64, error) {
	r := big.NewInt(0)
	err := self.reader.ReadContractWithABI(
		&r,
		self.Address,
		self.Abi,
		"required",
	)
	if err != nil {
		return 0, err
	}
	return r.Int64(), nil
}

func (self *MultisigContract) TransactionInfo(txid *big.Int) (address string, value *big.Int, data []byte, executed bool, confirmations []string, err error) {
	type response struct {
		Destination *common.Address
		Value       *big.Int
		Data        *[]byte
		Executed    *bool
	}
	ret := response{
		&common.Address{},
		big.NewInt(0),
		new([]byte),
		new(bool),
	}
	err = self.reader.ReadContractWithABI(
		&ret,
		self.Address,
		self.Abi,
		"transactions",
		txid,
	)
	if err != nil {
		return "", big.NewInt(0), []byte{}, false, []string{}, err
	}
	signers := []common.Address{}
	err = self.reader.ReadContractWithABI(
		&signers,
		self.Address,
		self.Abi,
		"getConfirmations",
		txid,
	)
	if err != nil {
		return "", big.NewInt(0), []byte{}, false, []string{}, err
	}
	confirmations = []string{}
	for _, s := range signers {
		confirmations = append(confirmations, s.Hex())
	}
	return ret.Destination.Hex(), ret.Value, *ret.Data, *ret.Executed, confirmations, nil
}

func NewMultisigContract(address string, network Network) (*MultisigContract, error) {
	reader, err := util.EthReader(network)
	if err != nil {
		return nil, err
	}

	a, err := abi.JSON(strings.NewReader(GNOSIS_MULTISIG_ABI))
	if err != nil {
		return nil, err
	}
	return &MultisigContract{
		address,
		network,
		reader,
		&a,
	}, nil
}
