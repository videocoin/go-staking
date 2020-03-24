package testtools

import (
	"crypto/ecdsa"
	"errors"
	"io/ioutil"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	// NetworkID id of the network we are using for testing, require for EIP155 signer
	NetworkID      uint64 = 1337
	ETHTransferGas uint64 = 21000
)

func DefaultNode() *Node {
	return new(Node).GenFaucet().WithDefaultConfig()
}

// DefaultConfig disables p2p stack and enables ipc communication using temporary directory.
func DefaultConfig() node.Config {
	cfg := node.Config{}
	ipc, err := ioutil.TempFile("", "e2enode-ipc-")
	if err != nil {
		panic(err.Error())
	}
	cfg.IPCPath = ipc.Name()
	cfg.HTTPModules = []string{"eth"}
	cfg.DataDir = ""
	cfg.NoUSB = true
	cfg.P2P.MaxPeers = 0
	cfg.P2P.ListenAddr = ":0"
	cfg.P2P.NoDiscovery = true
	cfg.P2P.DiscoveryV5 = false
	return cfg
}

type Node struct {
	config node.Config
	pkey   *ecdsa.PrivateKey

	mu     sync.Mutex
	node   *node.Node
	client *rpc.Client
}

func (n *Node) GenFaucet() *Node {
	pkey, err := crypto.GenerateKey()
	if err != nil {
		panic(err.Error())
	}
	return n.WithFaucet(pkey)
}

func (n *Node) WithFaucet(pkey *ecdsa.PrivateKey) *Node {
	n.pkey = pkey
	return n
}

func (n *Node) WithConfig(config node.Config) *Node {
	n.config = config
	return n
}

func (n *Node) WithDefaultConfig() *Node {
	n.config = DefaultConfig()
	return n
}

// Start the node with clique backend.
func (n *Node) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.node != nil {
		return errors.New("node already running")
	}
	stack, err := node.New(&n.config)
	if err != nil {
		return err
	}

	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	// ensure that etherbase is added to an account manager
	acc, err := ks.ImportECDSA(n.pkey, "")
	if err != nil {
		return err
	}
	err = ks.Unlock(acc, "")
	if err != nil {
		return err
	}

	ethcfg := eth.DefaultConfig
	ethcfg.NetworkId = NetworkID
	// 0 - start mining when transaction is pending
	ethcfg.Genesis = core.DeveloperGenesisBlock(1, crypto.PubkeyToAddress(n.pkey.PublicKey))
	stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		return eth.New(ctx, &ethcfg)
	})

	// start miner
	err = stack.Start()
	if err != nil {
		return err
	}
	var ethereum *eth.Ethereum
	err = stack.Service(&ethereum)
	if err != nil {
		return err
	}
	ethereum.TxPool().SetGasPrice(big.NewInt(1))
	if err := ethereum.StartMining(0); err != nil {
		return err
	}
	n.node = stack
	n.client, err = stack.Attach()
	return err
}

func (n *Node) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.node == nil {
		return errors.New("node not running")
	}
	n.node.Stop()
	n.node = nil
	return nil
}

func (n *Node) Client() *ethclient.Client {
	return ethclient.NewClient(n.client)
}

func (n *Node) FaucetService() Faucet {
	return NewFaucet(n.Client(), n.pkey)
}
