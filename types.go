package staking

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

//go:generate stringer -type=State
type State uint8

const (
	StateBonding State = iota
	StateBonded
	StateUnbonded
	StateUnbonding
)

type Transcoder struct {
	Address    common.Address
	State      State
	TotalStake *big.Int
	Capacity   *big.Int
}
