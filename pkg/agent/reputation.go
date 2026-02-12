package agent

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ABIs for ERC-8004 v2.0.0
const (
	identityABI = `[
		{"inputs":[{"internalType":"uint256","name":"agentId","type":"uint256"}],"name":"getAgentWallet","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
		{"inputs":[{"internalType":"uint256","name":"tokenId","type":"uint256"}],"name":"ownerOf","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},
		{"inputs":[{"internalType":"uint256","name":"agentId","type":"uint256"},{"internalType":"string","name":"metadataKey","type":"string"}],"name":"getMetadata","outputs":[{"internalType":"bytes","name":"","type":"bytes"}],"stateMutability":"view","type":"function"}
	]`
	reputationABI = `[
		{"inputs":[
			{"internalType":"uint256","name":"agentId","type":"uint256"},
			{"internalType":"address[]","name":"clientAddresses","type":"address[]"},
			{"internalType":"string","name":"tag1","type":"string"},
			{"internalType":"string","name":"tag2","type":"string"}
		],"name":"getSummary","outputs":[
			{"internalType":"uint64","name":"count","type":"uint64"},
			{"internalType":"int128","name":"summaryValue","type":"int128"},
			{"internalType":"uint8","name":"summaryValueDecimals","type":"uint8"}
		],"stateMutability":"view","type":"function"}
	]`
	validationABI = `[
		{"inputs":[
			{"internalType":"uint256","name":"agentId","type":"uint256"},
			{"internalType":"address[]","name":"validatorAddresses","type":"address[]"},
			{"internalType":"string","name":"tag","type":"string"}
		],"name":"getSummary","outputs":[
			{"internalType":"uint64","name":"count","type":"uint64"},
			{"internalType":"uint8","name":"avgResponse","type":"uint8"}
		],"stateMutability":"view","type":"function"}
	]`
)

// ERC8004Client provides methods to query the ERC-8004 v2.0.0 Registries on-chain.
type ERC8004Client struct {
	client       *ethclient.Client
	identityAddr common.Address
	reputAddr    common.Address
	validAddr    common.Address

	identityABI   abi.ABI
	reputationABI abi.ABI
	validationABI abi.ABI
}

func NewERC8004Client(rpcURL string, identityAddr, reputAddr, validAddr string) *ERC8004Client {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		fmt.Printf("[ERC8004] Failed to connect to RPC: %v\n", err)
		return nil
	}

	iABI, _ := abi.JSON(strings.NewReader(identityABI))
	rABI, _ := abi.JSON(strings.NewReader(reputationABI))
	vABI, _ := abi.JSON(strings.NewReader(validationABI))

	return &ERC8004Client{
		client:        client,
		identityAddr:  common.HexToAddress(identityAddr),
		reputAddr:     common.HexToAddress(reputAddr),
		validAddr:     common.HexToAddress(validAddr),
		identityABI:   iABI,
		reputationABI: rABI,
		validationABI: vABI,
	}
}

// GetAgentWallet returns the verified wallet address for an agent ID.
func (c *ERC8004Client) GetAgentWallet(agentId *big.Int) (common.Address, error) {
	data, _ := c.identityABI.Pack("getAgentWallet", agentId)
	res, err := c.call(c.identityAddr, data)
	if err != nil {
		return common.Address{}, err
	}
	var wallet common.Address
	err = c.identityABI.UnpackIntoInterface(&wallet, "getAgentWallet", res)
	return wallet, err
}

// GetMetadata retrieves a specific metadata value for an agent.
func (c *ERC8004Client) GetMetadata(agentId *big.Int, key string) (string, error) {
	data, err := c.identityABI.Pack("getMetadata", agentId, key)
	if err != nil {
		return "", err
	}
	res, err := c.call(c.identityAddr, data)
	if err != nil {
		return "", err
	}
	var val []byte
	err = c.identityABI.UnpackIntoInterface(&val, "getMetadata", res)
	return string(val), err
}

// GetAgentIdByWallet attempts to find an agent ID owned by a wallet by scanning logs.
func (c *ERC8004Client) GetAgentIdByWallet(wallet common.Address) (*big.Int, error) {
	// Registered(uint256 indexed agentId, string agentURI, address indexed owner)
	// Topic 0: Keccak256("Registered(uint256,string,address)")
	// Topic 2: address (indexed owner)
	sigHash := common.HexToHash("ca52e62c367d81bb2e328eb795f7c7ba24afb478408a26c0e201d155c449bc4a")

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(12345678), // Registry deployment block on Base Sepolia
		ToBlock:   nil,
		Addresses: []common.Address{c.identityAddr},
		Topics: [][]common.Hash{
			{sigHash},
			nil,
			{common.BytesToHash(wallet.Bytes())},
		},
	}

	logs, err := c.client.FilterLogs(context.Background(), query)
	if err != nil {
		return nil, fmt.Errorf("failed to filter registry logs: %w", err)
	}

	if len(logs) == 0 {
		return nil, fmt.Errorf("no agent identity NFT found for wallet %s in the registry", wallet.Hex())
	}

	// agentId is indexed, so it's in Topics[1]
	return new(big.Int).SetBytes(logs[len(logs)-1].Topics[1].Bytes()), nil
}

// GetReputationSummary returns aggregated signal for an agent.
func (c *ERC8004Client) GetReputationSummary(agentId *big.Int, tag1, tag2 string, querierAddr common.Address) (uint64, *big.Int, uint8, error) {
	// The client list should ideally contain the querier's address for personalized reputation,
	// or be used according to the specific consumer's logic.
	clients := []common.Address{querierAddr}

	data, err := c.reputationABI.Pack("getSummary", agentId, clients, tag1, tag2)
	if err != nil {
		return 0, nil, 0, err
	}
	res, err := c.call(c.reputAddr, data)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("reputation registry query failed: %w", err)
	}

	type Summary struct {
		Count                uint64
		SummaryValue         *big.Int
		SummaryValueDecimals uint8
	}
	var s Summary
	err = c.reputationABI.UnpackIntoInterface(&s, "getSummary", res)
	return s.Count, s.SummaryValue, s.SummaryValueDecimals, err
}

func (c *ERC8004Client) call(to common.Address, data []byte) ([]byte, error) {
	msg := ethereum.CallMsg{To: &to, Data: data}
	return c.client.CallContract(context.Background(), msg, nil)
}

func (c *ERC8004Client) Close() {
	if c.client != nil {
		c.client.Close()
	}
}
