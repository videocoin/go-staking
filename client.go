package staking

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/videocoin/go-protocol/staking"
)

var (
	one = big.NewInt(1)

	// ErrTransactionReverted raised when transaction was mined but has failed status.
	ErrTransactionReverted = errors.New("transaction reverted")
	// ErrInsufficientStake raised if delegated amount smaller than configured min delegation.
	ErrInsufficientStake = errors.New("insufficient stake")
	// ErrAlreadyRegistered raised if transcoder was already registered.
	ErrAlreadyRegistered = errors.New("already registered")
	// ErrNoPendingWithdrawals when there are not available withdrawals to complete.
	ErrNoPendingWithdrawals = errors.New("no pending withdrawals aviable")
)

// ETHBackend is a subset of ethereum rpc methods that are used in staking Client.
type ETHBackend interface {
	bind.ContractBackend
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
}

func NewClient(client ETHBackend, address common.Address) (*Client, error) {
	contract, err := staking.NewStakingManager(address, client)
	if err != nil {
		return nil, err
	}
	return &Client{
		client:   client,
		contract: contract,
	}, nil
}

type Client struct {
	client   ETHBackend
	contract *staking.StakingManager
}

func (c *Client) GetUnbondingPeriod(ctx context.Context) (*big.Int, error) {
	return c.contract.UnbondingPeriod(&bind.CallOpts{Context: ctx})
}

func (c *Client) GetMinDelegation(ctx context.Context) (*big.Int, error) {
	return c.contract.MinDelegation(&bind.CallOpts{Context: ctx})
}

func (c *Client) GetRequiredSelfStake(ctx context.Context) (*big.Int, error) {
	return c.contract.MinSelfStake(&bind.CallOpts{Context: ctx})
}

func (c *Client) IsTranscoderRegistered(ctx context.Context, address common.Address) (bool, error) {
	info, err := c.contract.Transcoders(&bind.CallOpts{Context: ctx}, address)
	if err != nil {
		return false, err
	}
	return info.Timestamp.Int64() != 0, nil
}

func (c *Client) GetTranscoderState(ctx context.Context, address common.Address) (State, error) {
	state, err := c.contract.GetTranscoderState(&bind.CallOpts{Context: ctx}, address)
	if err != nil {
		return 0, err
	}
	return State(state), nil
}

func (c *Client) GetTranscoderStake(ctx context.Context, address common.Address) (*big.Int, error) {
	return c.contract.GetTotalStake(&bind.CallOpts{Context: ctx}, address)
}

func (c *Client) GetDelegatorStake(ctx context.Context, transcoder, delegator common.Address) (*big.Int, error) {
	return c.contract.GetDelegatorStake(&bind.CallOpts{Context: ctx}, transcoder, delegator)
}

func (c *Client) GetTranscoderCapacity(ctx context.Context, address common.Address) (*big.Int, error) {
	info, err := c.contract.Transcoders(&bind.CallOpts{Context: ctx}, address)
	if err != nil {
		return nil, err
	}
	return info.Capacity, nil
}

func (c *Client) GetTranscoder(ctx context.Context, address common.Address) (tcr Transcoder, err error) {
	info, err := c.contract.Transcoders(&bind.CallOpts{Context: ctx}, address)
	if err != nil {
		return tcr, err
	}
	state, err := c.GetTranscoderState(ctx, address)
	if err != nil {
		return tcr, err
	}
	selfStake, err := c.contract.GetSelfStake(&bind.CallOpts{Context: ctx}, address)
	if err != nil {
		return tcr, err
	}
	delegated := new(big.Int).Sub(info.Total, selfStake)
	timestamp := info.Timestamp.Uint64()
	return Transcoder{
		Address:        address,
		TotalStake:     info.Total,
		SelfStake:      selfStake,
		DelegatedStake: delegated,
		Capacity:       info.Capacity,
		State:          state,
		Timestamp:      timestamp,
	}, nil
}

func (c *Client) TranscodersCount(ctx context.Context) (*big.Int, error) {
	return c.contract.TranscodersCount(&bind.CallOpts{Context: ctx})
}

func (c *Client) GetTranscoderAt(ctx context.Context, index *big.Int) (tcr Transcoder, err error) {
	address, err := c.contract.TranscodersArray(&bind.CallOpts{Context: ctx}, index)
	if err != nil {
		return tcr, err
	}
	return c.GetTranscoder(ctx, address)
}

func (c *Client) TranscoderIterator(ctx context.Context) (*TranscoderIterator, error) {
	count, err := c.TranscodersCount(ctx)
	if err != nil {
		return nil, err
	}
	return newTranscoderIterator(c, new(big.Int), count), nil
}

func (c *Client) GetAllTranscoders(ctx context.Context) (tcrs []Transcoder, err error) {
	iter, err := c.TranscoderIterator(ctx)
	if err != nil {
		return nil, err
	}
	for iter.Next(ctx) {
		tcrs = append(tcrs, iter.Current())
	}
	if iter.Error() != nil {
		return nil, iter.Error()
	}
	return tcrs, nil
}

func (c *Client) GetBondedTranscoders(ctx context.Context) (tcrs []Transcoder, err error) {
	iter, err := c.TranscoderIterator(ctx)
	if err != nil {
		return nil, err
	}
	for iter.Next(ctx) {
		tcr := iter.Current()
		if tcr.State != StateBonded {
			continue
		}
		tcrs = append(tcrs, tcr)
	}
	if iter.Error() != nil {
		return nil, iter.Error()
	}
	return tcrs, nil
}

// HeadTimestamp returns timestamp of the head block. Can be used to compare with various timestamp
// returned to the caller, e.g. transcoder Timestamp or withdrawal ReadinessTimestamp.
func (c *Client) HeadTimestamp(ctx context.Context) (uint64, error) {
	header, err := c.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, err
	}
	return header.Time, nil
}

func (c *Client) Delegate(ctx context.Context, key *ecdsa.PrivateKey, to common.Address, amount *big.Int) error {
	min, err := c.GetMinDelegation(ctx)
	if err != nil {
		return err
	}
	if amount.Cmp(min) < 0 {
		return fmt.Errorf("%w: amount %v is smaller than required delegation %v",
			ErrInsufficientStake, amount, min)
	}
	opts := bind.NewKeyedTransactor(key)
	opts.Context = ctx
	opts.Value = amount
	tx, err := c.contract.Delegate(opts, to)
	if err != nil {
		return err
	}
	receipt, err := bind.WaitMined(ctx, c.client, tx)
	if err != nil {
		return err
	}
	if receipt.Status == types.ReceiptStatusFailed {
		return fmt.Errorf("%w: failed to delegate from 0x%x to 0x%x. amount %v",
			ErrTransactionReverted, opts.From, to, amount,
		)
	}
	return nil
}

// RegisterTranscoder ensures that transcoder is registered. If transcoder already registered
// new reward rate is not applied.
func (c *Client) RegisterTranscoder(ctx context.Context, key *ecdsa.PrivateKey, rewardRate uint64) error {
	opts := bind.NewKeyedTransactor(key)
	opts.Context = ctx
	reg, err := c.IsTranscoderRegistered(ctx, opts.From)
	if err != nil {
		return err
	}
	if reg {
		return fmt.Errorf("0x%x %w", opts.From, ErrAlreadyRegistered)
	}
	tx, err := c.contract.RegisterTranscoder(opts, new(big.Int).SetUint64(rewardRate))
	if err != nil {
		return err
	}
	receipt, err := bind.WaitMined(ctx, c.client, tx)
	if err != nil {
		return err
	}
	if receipt.Status == types.ReceiptStatusFailed {
		return fmt.Errorf("%w: failed to register 0x%x with rate %d",
			ErrTransactionReverted, opts.From, rewardRate)
	}
	return nil
}

// RequestWithdrawal either creates pending withdrawal that can be completed after ReadinessTimestamp
// or completes withdrawal immediatly if transcoder is not BONDED/UNBONDING. In the latter case Amount will be non-nil.
// And ReadinessTimestamp is 0.
func (c *Client) RequestWithdrawal(ctx context.Context,
	key *ecdsa.PrivateKey,
	from common.Address,
	amount *big.Int) (info WithdrawalInfo, err error) {
	opts := bind.NewKeyedTransactor(key)
	delegated, err := c.contract.GetDelegatorStake(&bind.CallOpts{Context: ctx}, from, opts.From)
	if err != nil {
		return info, err
	}
	if delegated.Cmp(amount) < 0 {
		return info, fmt.Errorf("can't withdraw. delegated amount %v is less than requested %v", delegated, amount)
	}
	tx, err := c.contract.RequestUnbonding(opts, from, amount)
	if err != nil {
		return info, err
	}
	receipt, err := bind.WaitMined(ctx, c.client, tx)
	if err != nil {
		return info, err
	}
	if receipt.Status == types.ReceiptStatusFailed {
		return info, fmt.Errorf("%w: failed to request withdrawal from 0x%x. amount %v",
			ErrTransactionReverted, from, amount,
		)
	}
	if len(receipt.Logs) > 0 {
		unbonding, err := c.contract.ParseUnbondingRequested(*receipt.Logs[0])
		if err != nil {
			return info, err
		}
		info.ReadinessTimestamp = unbonding.Readiness.Uint64()
	}
	if len(receipt.Logs) > 1 {
		withdraw, err := c.contract.ParseStakeWithdrawal(*receipt.Logs[1])
		if err != nil {
			return info, err
		}
		info.Amount = withdraw.Amount
		info.ReadinessTimestamp = 0 // set timestamp to 0 cause request was already completed.
	}
	return info, nil
}

// CompleteWithdrawals completes all pending withdrawals, if any are available. All amounts from withdrawals
// are accumulated into info.Amount.
func (c *Client) CompleteWithdrawals(ctx context.Context, key *ecdsa.PrivateKey) (info WithdrawalInfo, err error) {
	opts := bind.NewKeyedTransactor(key)
	pending, err := c.contract.PendingWithdrawalsExist(&bind.CallOpts{Context: ctx, From: opts.From})
	if err != nil {
		return info, err
	}
	if !pending {
		return info, ErrNoPendingWithdrawals
	}
	tx, err := c.contract.WithdrawAllPending(opts)
	if err != nil {
		return info, err
	}
	receipt, err := bind.WaitMined(ctx, c.client, tx)
	if err != nil {
		return info, err
	}
	if receipt.Status == types.ReceiptStatusFailed {
		return info, fmt.Errorf("%w: failed to complete pending withdrawals", ErrTransactionReverted)
	}
	amount := big.NewInt(0)
	for _, log := range receipt.Logs {
		withdraw, err := c.contract.ParseStakeWithdrawal(*log)
		if err != nil {
			return info, err
		}
		amount = amount.Add(amount, withdraw.Amount)
	}
	info.Amount = amount
	return info, err
}

// WaitWithdrawalsCompleted exits either when some withdrawals were completed or by context timeout. It should not be executed
// concurrently with another WaitWithdrawalsCompleted/CompleteWithdrawals.
func (c *Client) WaitWithdrawalsCompleted(ctx context.Context, key *ecdsa.PrivateKey) (info WithdrawalInfo, err error) {
	info, err = c.CompleteWithdrawals(ctx, key)
	if err == nil {
		return info, err
	}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return info, ctx.Err()
		case <-ticker.C:
			info, err = c.CompleteWithdrawals(ctx, key)
			if err == nil {
				return info, nil
			}
		}
	}
}

func newTranscoderIterator(client *Client, start, end *big.Int) *TranscoderIterator {
	return &TranscoderIterator{
		client: client,
		start:  start,
		end:    end,
	}
}

type TranscoderIterator struct {
	client *Client

	start, end *big.Int

	transcoder Transcoder
	err        error
}

func (iter *TranscoderIterator) Next(ctx context.Context) bool {
	if iter.start.Cmp(iter.end) >= 0 || iter.err != nil {
		return false
	}
	tcr, err := iter.client.GetTranscoderAt(ctx, iter.start)
	iter.err = err
	if err != nil {
		return false
	}
	iter.transcoder = tcr
	iter.start.Add(iter.start, one)
	return true
}

func (iter *TranscoderIterator) Current() Transcoder {
	return iter.transcoder
}

func (iter *TranscoderIterator) Error() error {
	return iter.err
}
