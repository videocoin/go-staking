package main

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/kelseyhightower/envconfig"
	"github.com/videocoin/common/crypto"
	"github.com/videocoin/go-protocol/staking"
)

type config struct {
	Key      string
	Password string
	URL      string
	Contract common.Address

	UpdateApproval bool
	ApprovalPeriod time.Duration
	UpdateMinStake bool
	MinStake       string

	Slashed []common.Address
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	var c config
	must(envconfig.Process("eth", &c))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := ethclient.Dial(c.URL)
	must(err)
	contract, err := staking.NewStakingManager(c.Contract, client)
	must(err)
	key, err := crypto.DecryptKeyFile(c.Key, c.Password)
	must(err)
	opts := bind.NewKeyedTransactor(key.PrivateKey)
	opts.Context = ctx
	if c.UpdateApproval {
		tx, err := contract.SetApprovalPeriod(opts, new(big.Int).SetUint64(uint64(c.ApprovalPeriod.Seconds())))
		must(err)
		receipt, err := bind.WaitMined(ctx, client, tx)
		must(err)
		if receipt.Status == types.ReceiptStatusFailed {
			panic("failed to set approval period")
		}
		fmt.Printf("updated approval period to %v\n", c.ApprovalPeriod)
	}
	if c.UpdateMinStake {
		stake, ok := new(big.Int).SetString(c.MinStake, 0)
		if !ok {
			panic(fmt.Sprintf("can't use %s as math.BigInt", c.MinStake))
		}
		tx, err := contract.SetSelfMinStake(opts, stake)
		must(err)
		receipt, err := bind.WaitMined(ctx, client, tx)
		must(err)
		if receipt.Status == types.ReceiptStatusFailed {
			panic("failed to set min stake")
		}
		fmt.Printf("updated min stake to %v\n", stake)
	}
	for _, address := range c.Slashed {
		tx, err := contract.Slash(opts, address)
		must(err)
		receipt, err := bind.WaitMined(ctx, client, tx)
		must(err)
		if receipt.Status == types.ReceiptStatusFailed {
			panic(fmt.Sprintf("failed to jail %s", address.String()))
		}
		fmt.Printf("jailed %s\n", address.String())
	}
}
