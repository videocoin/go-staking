package staking

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/videocoin/go-protocol/staking"
)

var (
	one = big.NewInt(1)
)

func Dial(rawurl string, address common.Address) (*Client, error) {
	client, err := ethclient.Dial(rawurl)
	if err != nil {
		return nil, err
	}
	contract, err := staking.NewStakingManager(address, client)
	if err != nil {
		return nil, err
	}
	return NewClient(contract), nil
}

func NewClient(contract *staking.StakingManager) *Client {
	return &Client{
		contract: contract,
	}
}

type Client struct {
	contract *staking.StakingManager
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
	return Transcoder{
		Address:    address,
		TotalStake: info.Total,
		Capacity:   info.Capacity,
		State:      state,
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
