package staking

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
	"github.com/videocoin/go-contracts/bindings/staking"
)

type StakingSuite struct {
	suite.Suite

	FundedKeys      []*ecdsa.PrivateKey
	Backend         *backends.SimulatedBackend
	Contract        *staking.StakingManager
	ContractAddress common.Address
}

func (s *StakingSuite) SetupTest() {
	alloc := core.GenesisAlloc{}
	for i := 0; i < 20; i++ {
		pkey, err := crypto.GenerateKey()
		s.Require().NoError(err)
		opts := bind.NewKeyedTransactor(pkey)
		s.FundedKeys = append(s.FundedKeys, pkey)
		alloc[opts.From] = core.GenesisAccount{Balance: new(big.Int).SetUint64(^uint64(0))}
	}

	s.Backend = backends.NewSimulatedBackend(alloc, ^uint64(0))

	address, _, contract, err := staking.DeployStakingManager(bind.NewKeyedTransactor(s.FundedKeys[0]),
		s.Backend,
		big.NewInt(10),
		big.NewInt(100),
		big.NewInt(0),
		big.NewInt(0),
		big.NewInt(0),
		common.Address{2, 2, 2},
	)
	s.Require().NoError(err)
	s.Backend.Commit()
	s.Contract = contract
	s.ContractAddress = address
}

func (s *StakingSuite) TearDownTest() {
	s.FundedKeys = nil
}

func TestClient(t *testing.T) {
	suite.Run(t, new(ClientSuite))
}

type ClientSuite struct {
	StakingSuite

	ctx    context.Context
	cancel func()

	StakingClient *Client
}

func (s *ClientSuite) SetupTest() {
	s.StakingSuite.SetupTest()
	client, err := NewClient(s.Backend, s.ContractAddress)
	s.Require().NoError(err)
	s.StakingClient = client
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			default:
				s.Backend.Commit()
			}
		}
	}()
}

func (s *ClientSuite) TeatDownTest() {
	s.StakingSuite.TearDownTest()
	s.cancel()
}

func (s *ClientSuite) TestGetTrascoders() {
	for _, pkey := range s.FundedKeys {
		err := s.StakingClient.RegisterTranscoder(context.Background(), pkey, 10)
		s.Require().NoError(err)
	}

	transcoders, err := s.StakingClient.GetAllTranscoders(context.Background())
	s.Require().NoError(err)
	s.Require().Len(transcoders, len(s.FundedKeys))

	for i := range transcoders {
		opts := bind.NewKeyedTransactor(s.FundedKeys[i])
		s.Require().Equal(opts.From, transcoders[i].Address)
		s.Require().Equal(StateBonding, transcoders[i].State)
	}
}

func (s *ClientSuite) TestTranscoderWithdraw() {
	transcoder := s.FundedKeys[0]

	err := s.StakingClient.RegisterTranscoder(s.ctx, transcoder, 10)
	s.Require().NoError(err)

	// delegate enough funds to transition to bonded state
	addr := crypto.PubkeyToAddress(transcoder.PublicKey)
	err = s.StakingClient.Delegate(s.ctx, transcoder, addr, big.NewInt(1e15))
	s.Require().NoError(err)

	state, err := s.StakingClient.GetTranscoderState(context.TODO(), addr)
	s.Require().NoError(err)
	s.Require().Equal(StateBonded, state)

	amount := big.NewInt(1e14)
	info, err := s.StakingClient.RequestWithdrawal(s.ctx, transcoder, addr, amount)
	s.Require().NoError(err)
	s.Require().Nil(info.Amount)
	s.Require().NotEqual(info.ReadinessTimestamp, 0)

	info, err = s.StakingClient.CompleteWithdrawals(s.ctx, transcoder)
	s.Require().NoError(err)
	s.Require().NotNil(info.Amount)
	s.Require().Equal(amount.Int64(), info.Amount.Int64())

	_, err = s.StakingClient.CompleteWithdrawals(s.ctx, transcoder)
	s.Require().True(errors.Is(err, ErrNoPendingWithdrawals))
}

func (s *ClientSuite) TestRequestCompletedImmediatly() {
	transcoder := s.FundedKeys[0]

	err := s.StakingClient.RegisterTranscoder(s.ctx, transcoder, 10)
	s.Require().NoError(err)

	amount := big.NewInt(50)

	// delegate enough funds to transition to bonded state
	addr := crypto.PubkeyToAddress(transcoder.PublicKey)
	err = s.StakingClient.Delegate(s.ctx, transcoder, addr, amount)
	s.Require().NoError(err)

	info, err := s.StakingClient.RequestWithdrawal(s.ctx, transcoder, addr, amount)
	s.Require().NoError(err)
	s.Require().Equal(amount.Int64(), info.Amount.Int64())
	s.Require().Empty(info.ReadinessTimestamp)
}

func (s *ClientSuite) TestDelegatedStake() {
	err := s.StakingClient.RegisterTranscoder(s.ctx, s.FundedKeys[0], 10)
	s.Require().NoError(err)

	amount := big.NewInt(50)

	// delegate enough funds to transition to bonded state
	addr := crypto.PubkeyToAddress(s.FundedKeys[0].PublicKey)
	err = s.StakingClient.Delegate(s.ctx, s.FundedKeys[1], addr, amount)
	s.Require().NoError(err)

	transcoder, err := s.StakingClient.GetTranscoder(s.ctx, addr)
	s.Require().NoError(err)
	s.Require().Empty(transcoder.SelfStake.Int64())
	s.Require().Equal(amount.Int64(), transcoder.DelegatedStake.Int64())
	s.Require().Equal(amount.Int64(), transcoder.TotalStake.Int64())
}

func (s *ClientSuite) TestCompleteMultiple() {
	err := s.StakingClient.RegisterTranscoder(s.ctx, s.FundedKeys[0], 10)
	s.Require().NoError(err)

	amount := big.NewInt(1000)

	// delegate enough funds to transition to bonded state
	addr := crypto.PubkeyToAddress(s.FundedKeys[0].PublicKey)
	err = s.StakingClient.Delegate(s.ctx, s.FundedKeys[0], addr, amount)
	s.Require().NoError(err)

	for i := 0; i < 4; i++ {
		info, err := s.StakingClient.RequestWithdrawal(s.ctx, s.FundedKeys[0], addr, big.NewInt(250))
		s.Require().NoError(err)
		s.Require().NotEmpty(info.ReadinessTimestamp)
	}

	info, err := s.StakingClient.CompleteWithdrawals(s.ctx, s.FundedKeys[0])
	s.Require().NoError(err)
	s.Require().Equal(amount.Int64(), info.Amount.Int64())
}

func (s *ClientSuite) TestTransitionToBondedWithSeveralDelegates() {
	err := s.StakingClient.RegisterTranscoder(s.ctx, s.FundedKeys[0], 10)
	s.Require().NoError(err)
	addr := crypto.PubkeyToAddress(s.FundedKeys[0].PublicKey)
	for i := 0; i < 3; i++ {
		err := s.StakingClient.Delegate(s.ctx, s.FundedKeys[0], addr, big.NewInt(40))
		s.Require().NoError(err)
	}
	transcoders, err := s.StakingClient.GetBondedTranscoders(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(transcoders, 1)
	s.Require().Equal(addr, transcoders[0].Address)
}

func (s *ClientSuite) TestGetUnregisteredTranscoderState() {
	state, err := s.StakingClient.GetTranscoderState(s.ctx, common.Address{1, 2})
	s.Require().NoError(err)
	s.Require().Equal(StateUnregistered, state)
}

func (s *ClientSuite) TestWaitWithdrawalCompletedTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := s.StakingClient.WaitWithdrawalsCompleted(ctx, s.FundedKeys[0])
	s.Require().True(errors.Is(err, context.DeadlineExceeded))
}

func (s *ClientSuite) TestWaithWithdrawalCompletedSuccess() {
	var (
		transcoder  = s.FundedKeys[0]
		infos       = make(chan WithdrawalInfo, 1)
		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	)
	defer cancel()
	go func() {
		info, err := s.StakingClient.WaitWithdrawalsCompleted(ctx, s.FundedKeys[0])
		s.Require().NoError(err)
		infos <- info
	}()

	err := s.StakingClient.RegisterTranscoder(s.ctx, transcoder, 10)
	s.Require().NoError(err)

	// delegate enough funds to transition to bonded state
	addr := crypto.PubkeyToAddress(transcoder.PublicKey)
	err = s.StakingClient.Delegate(s.ctx, transcoder, addr, big.NewInt(1e15))
	s.Require().NoError(err)

	amount := big.NewInt(1e15)
	info, err := s.StakingClient.RequestWithdrawal(s.ctx, transcoder, addr, amount)
	s.Require().NoError(err)
	s.Require().Nil(info.Amount)
	s.Require().NotEqual(info.ReadinessTimestamp, 0)

	select {
	case <-ctx.Done():
		s.Require().FailNow(ctx.Err().Error())
	case info := <-infos:
		s.Require().Equal(amount.Int64(), info.Amount.Int64())
	}
}

func (s *ClientSuite) TestEffectiveMinSelfStake() {
	s.Require().NoError(s.StakingClient.RegisterTranscoder(s.ctx, s.FundedKeys[0], 10))
	addr := crypto.PubkeyToAddress(s.FundedKeys[0].PublicKey)
	original := big.NewInt(100)
	s.Require().NoError(s.StakingClient.Delegate(s.ctx, s.FundedKeys[0], addr, original))

	transcoders, err := s.StakingClient.GetBondedTranscoders(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(transcoders, 1)

	opts := bind.NewKeyedTransactor(s.FundedKeys[0])
	_, err = s.Contract.SetSelfMinStake(opts, big.NewInt(1000))
	s.Require().NoError(err)

	transcoders, err = s.StakingClient.GetBondedTranscoders(s.ctx)
	s.Require().NoError(err)
	s.Require().Len(transcoders, 1)
	s.Require().Equal(original, transcoders[0].EffectiveMinSelfStake)
}
