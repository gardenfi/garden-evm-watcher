package watcher_test

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/catalogfi/blockchain/evm/bindings/contracts/htlc/gardenhtlc"
	"github.com/catalogfi/blockchain/evm/bindings/openzeppelin/contracts/token/ERC20/erc20"
	"github.com/catalogfi/garden-evm-watcher/pkg/watcher"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestWatcher(t *testing.T) {
	// Set up test environment
	logger := zaptest.NewLogger(t)

	

	// Deploy token contract (for simplicity, we'll use a dummy token)
	tokenAddress, _, token, err := erc20.DeployERC20

	// Deploy GardenHTLC contract
	htlcAddress, _, htlc, err := gardenhtlc.DeployGardenHTLC(auth, simulatedClient, tokenAddress, "GardenHTLC", "v1")
	require.NoError(t, err)

	// Approve tokens for HTLC contract
	_, err = token.Approve(auth, htlcAddress, big.NewInt(1000000))
	require.NoError(t, err)

	// Parse ABI
	parsedABI, err := abi.JSON(strings.NewReader(gardenhtlc.GardenHTLCMetaData.ABI))
	require.NoError(t, err)

	// Create contract config
	contractConfig, err := watcher.NewContractConfig(htlcAddress, parsedABI, map[string]watcher.EventHandler{
		"Initiated": logHandler(logger, "Initiated"),
		"Redeemed":  logHandler(logger, "Redeemed"),
		"Refunded":  logHandler(logger, "Refunded"),
	})
	require.NoError(t, err)

	// Create watcher config
	config := watcher.WatcherConfig{
		EthClient:       (*ethclient.Client)(simulatedClient),
		Contracts:       []watcher.ContractConfig{contractConfig},
		MaxBlockSpan:    1000,
		PollingInterval: 100 * time.Millisecond,
		Logger:          logger,
		WorkerCount:     1,
		QueueSize:       100,
	}

	// Create and start watcher
	w, err := watcher.NewWatcher(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		err := w.Start(ctx)
		assert.NoError(t, err)
	}()

	// Perform HTLC operations
	secretHash := [32]byte{1, 2, 3}
	secret := []byte("secret")
	timelock := big.NewInt(time.Now().Add(1 * time.Hour).Unix())
	amount := big.NewInt(1000)

	// Initiate
	tx, err := htlc.Initiate(auth, auth.From, timelock, amount, secretHash)
	require.NoError(t, err)

	receipt, err := simulatedClient.TransactionReceipt(context.Background(), tx.Hash())
	require.NoError(t, err)
	orderID := receipt.Logs[0].Topics[1]

	// Redeem
	_, err = htlc.Redeem(auth, orderID, secret)
	require.NoError(t, err)

	// Create a new order for refund
	timelock = big.NewInt(time.Now().Add(-1 * time.Hour).Unix())
	tx, err = htlc.Initiate(auth, auth.From, timelock, amount, secretHash)
	require.NoError(t, err)

	receipt, err = simulatedClient.TransactionReceipt(context.Background(), tx.Hash())
	require.NoError(t, err)
	refundOrderID := receipt.Logs[0].Topics[1]

	// Refund
	_, err = htlc.Refund(auth, refundOrderID)
	require.NoError(t, err)

	// Wait for events to be processed
	time.Sleep(1 * time.Second)

	// Add assertions here to verify that events were properly handled
	// For example, you could check logs or use a mock logger to verify that the correct events were logged
}

func logHandler(logger *zap.Logger, eventName string) watcher.EventHandler {
	return func(log types.Log) error {
		logger.Info("Event handled", zap.String("event", eventName), zap.Any("log", log))
		return nil
	}
}


