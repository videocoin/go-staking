package staking

import (
	"context"
	"math/big"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
	"github.com/videocoin/go-protocol/staking"
	"github.com/videocoin/go-staking/testtools"
)

type StakingSuite struct {
	suite.Suite

	Node     *testtools.Node
	Client   *ethclient.Client
	Contract *staking.StakingManager
}

func (s *StakingSuite) SetupTest() {
	ctx := context.TODO()

	node := testtools.DefaultNode()
	s.Require().NoError(node.Start())

	s.Node = node
	client := node.Client()
	s.Client = client

	owner, err := crypto.GenerateKey()
	s.Require().NoError(err)

	tx, err := node.FaucetService().Request(ctx, crypto.PubkeyToAddress(owner.PublicKey), new(big.Int).SetUint64(1e17))
	s.Require().NoError(err)
	_, err = bind.WaitMined(ctx, client, tx)
	s.Require().NoError(err)

	auth := bind.NewKeyedTransactor(owner)

	_, tx, contract, err := staking.DeployStakingManager(auth,
		client,
		big.NewInt(1),
		big.NewInt(100),
		big.NewInt(0),
		big.NewInt(1000000),
		big.NewInt(0),
		common.Address{2, 2, 2},
	)
	s.Require().NoError(err)
	_, err = bind.WaitMined(ctx, client, tx)
	s.Require().NoError(err)
	s.Contract = contract
}

func (s *StakingSuite) TearDownTest() {
	s.NoError(s.Node.Stop()) // assert is intentional
}

func TestClient(t *testing.T) {
	suite.Run(t, new(ClientSuite))
}

type ClientSuite struct {
	StakingSuite

	FundedKeys    []*bind.TransactOpts
	StakingClient *Client
}

func (s *ClientSuite) SetupTest() {
	s.StakingSuite.SetupTest()
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	)
	defer cancel()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pkey, err := crypto.GenerateKey()
			s.Require().NoError(err)
			auth := bind.NewKeyedTransactor(pkey)
			mu.Lock()
			tx, err := s.Node.FaucetService().Request(ctx, auth.From, big.NewInt(1e18))
			mu.Unlock()
			s.Require().NoError(err)
			receipt, err := bind.WaitMined(ctx, s.Client, tx)
			s.Require().NoError(err)
			s.Require().Equal(types.ReceiptStatusSuccessful, receipt.Status)
			mu.Lock()
			s.FundedKeys = append(s.FundedKeys, auth)
			mu.Unlock()
		}()
	}
	wg.Wait()
	// sort keys alphabetically for easier comparison
	sort.Slice(s.FundedKeys, func(i, j int) bool {
		return s.FundedKeys[i].From.String() < s.FundedKeys[j].From.String()
	})

	s.StakingClient = NewClient(s.Contract)
}

func (s *ClientSuite) TeatDownTest() {
	s.StakingSuite.TearDownTest()
	s.FundedKeys = nil
}

func (s *ClientSuite) TestGetTranscoders() {
	txs := []*types.Transaction{}
	for _, opts := range s.FundedKeys {
		tx, err := s.Contract.RegisterTranscoder(opts, big.NewInt(17))
		s.Require().NoError(err)
		txs = append(txs, tx)
	}
	_, err := bind.WaitMined(context.TODO(), s.Client, txs[len(txs)-1])
	s.Require().NoError(err)

	transcoders, err := s.StakingClient.GetAllTranscoders(context.Background())
	s.Require().NoError(err)
	s.Require().Len(transcoders, len(s.FundedKeys))
	sort.Slice(transcoders, func(i, j int) bool {
		return transcoders[i].Address.String() < transcoders[j].Address.String()
	})
	for i := range transcoders {
		s.Require().Equal(s.FundedKeys[i].From, transcoders[i].Address)
		s.Require().Equal(StateBonding, transcoders[i].State)
	}
}
