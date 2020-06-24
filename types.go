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
	StateUnregistered
)

type Transcoder struct {
	Address    common.Address
	State      State
	TotalStake *big.Int
	SelfStake  *big.Int
	// DelegatedStake is a stake delegated to transcoder.
	DelegatedStake *big.Int
	Capacity       *big.Int
	// Timestamp is registration time in seconds.
	Timestamp uint64
	// EffectiveMinSelfStake is a global MinSelfStake parameter that was effective when transcoder registered.
	EffectiveMinSelfStake *big.Int
}

type WithdrawalInfo struct {
	// ReadinessTimestamp when head block timestamp will be equal or higher to
	// to this timestamp requested unbonding will be available for withdraw.
	ReadinessTimestamp uint64
	// Amount will be non-nil if requested unbonding was withdrawen immediatly.
	// Which is the case when transcoder is not BONDED or UNBONDING.
	Amount *big.Int
}
