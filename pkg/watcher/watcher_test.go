package watcher_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/catalogfi/blockchain/evm/bindings/contracts/htlc/gardenhtlc"
	"github.com/catalogfi/garden-evm-watcher/pkg/bindings"
	htlcConfig "github.com/catalogfi/garden-evm-watcher/pkg/handlers/htlc"
	"github.com/catalogfi/garden-evm-watcher/pkg/model"
	"github.com/catalogfi/garden-evm-watcher/pkg/store"
	"github.com/catalogfi/garden-evm-watcher/pkg/watcher"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestWatcher(t *testing.T) {
	logger, sugar := setupLogger(t)
	defer logger.Sync()

	client := setupEthClient(t)
	transactor, aliceAddress, bobAddress := setupAccounts(t, client)

	tokenAddr, token := deployTestERC20(t, client, transactor, sugar)
	htlcAddress, htlc := deployGardenHTLC(t, client, transactor, tokenAddr, sugar)

	approveTokens(t, client, transactor, token, htlcAddress, sugar)

	w := setupWatcher(t, client, htlcAddress, logger)

	watcherCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pgDB, cleanup := SetupTestDB(t)
	defer cleanup()

	db := store.NewStore(pgDB)

	ctx := store.ContextWithStore(watcherCtx, db)

	sugar.Info("Starting watcher")
	err := w.Start(ctx)
	assert.NoError(t, err)

	testInitiateAndRedeem(t, client, transactor, htlc, aliceAddress, bobAddress, sugar, db)
	testInitiateAndRefund(t, client, transactor, htlc, aliceAddress, bobAddress, sugar, db)

	sugar.Info("Test completed, stopping watcher")
	w.Stop()
	sugar.Info("Test finished")
}

func SetupTestDB(t *testing.T) (*gorm.DB, func()) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		t.Fatalf("Could not connect to docker: %s", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "15",
		Env: []string{
			"POSTGRES_PASSWORD=secret",
			"POSTGRES_DB=testdb",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})
	if err != nil {
		t.Fatalf("Could not start resource: %s", err)
	}

	hostAndPort := resource.GetHostPort("5432/tcp")
	databaseUrl := fmt.Sprintf("postgres://postgres:secret@%s/testdb?sslmode=disable", hostAndPort)

	var db *gorm.DB
	if err = pool.Retry(func() error {
		var err error
		db, err = gorm.Open(postgres.Open(databaseUrl), &gorm.Config{})
		if err != nil {
			return err
		}
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Ping()
	}); err != nil {
		t.Fatalf("Could not connect to docker: %s", err)
	}

	// Migrate your schema
	err = db.AutoMigrate(
		&model.CreateOrder{},
		&model.FillOrder{},
		&model.Swap{},
		&model.MatchedOrder{},
		&model.Strategy{},
	) // Add all your models here
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	cleanup := func() {
		t.Log("Cleaning up the test database")
		sqlDB, err := db.DB()
		if err != nil {
			t.Logf("Error getting database instance: %v", err)
		} else {
			sqlDB.Close()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = resource.Expire(1) // This will stop and remove the container
		if err != nil {
			t.Logf("Could not expire the resource: %s", err)
		}

		if err := pool.Purge(resource); err != nil {
			t.Logf("Could not purge resource: %s", err)
		}

		expectedErr := &docker.NoSuchContainer{ID: resource.Container.ID}
		// Wait for the container to actually be removed
		if err := pool.Client.RemoveContainer(docker.RemoveContainerOptions{
			ID:      resource.Container.ID,
			Force:   true,
			Context: ctx,
		}); err.Error() == expectedErr.Error() {
			t.Logf("Container successfully removed")
		} else if err != nil {
			t.Logf("Error removing container: %v", err)
		}
	}

	return db, cleanup
}

func setupLogger(t *testing.T) (*zap.Logger, *zap.SugaredLogger) {
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	sugar := zaptest.NewLogger(t).Sugar()
	return logger, sugar
}

func setupEthClient(t *testing.T) *ethclient.Client {
	url := os.Getenv("ETH_URL")
	client, err := ethclient.Dial(url)
	require.NoError(t, err)
	return client
}

func setupAccounts(t *testing.T, client *ethclient.Client) (*bind.TransactOpts, common.Address, common.Address) {
	chainID, err := client.ChainID(context.Background())
	require.NoError(t, err)

	key := getPrivateKey(t, "ETH_KEY_1")
	keyBob := getPrivateKey(t, "ETH_KEY_2")

	transactor, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	require.NoError(t, err)

	bobAddress := crypto.PubkeyToAddress(keyBob.PublicKey)
	aliceAddress := crypto.PubkeyToAddress(key.PublicKey)

	return transactor, aliceAddress, bobAddress
}

func getPrivateKey(t *testing.T, envVar string) *ecdsa.PrivateKey {
	keyStr := strings.TrimPrefix(os.Getenv(envVar), "0x")
	keyBytes, err := hex.DecodeString(keyStr)
	require.NoError(t, err)
	key, err := crypto.ToECDSA(keyBytes)
	require.NoError(t, err)
	return key
}

func deployTestERC20(t *testing.T, client *ethclient.Client, transactor *bind.TransactOpts, sugar *zap.SugaredLogger) (common.Address, *bindings.TestERC20) {
	sugar.Info("Deploying TestERC20 token")
	tokenAddr, tx, token, err := bindings.DeployTestERC20(transactor, client)
	require.NoError(t, err)
	sugar.Infof("TestERC20 deployment transaction sent: %s", tx.Hash().Hex())

	_, err = bind.WaitMined(context.Background(), client, tx)
	require.NoError(t, err)
	sugar.Infof("TestERC20 token deployed at: %s", tokenAddr.Hex())

	return tokenAddr, token
}

func deployGardenHTLC(t *testing.T, client *ethclient.Client, transactor *bind.TransactOpts, tokenAddr common.Address, sugar *zap.SugaredLogger) (common.Address, *gardenhtlc.GardenHTLC) {
	sugar.Info("Deploying GardenHTLC contract")
	htlcAddress, tx, htlc, err := gardenhtlc.DeployGardenHTLC(transactor, client, tokenAddr, "GardenHtlc", "V1")
	require.NoError(t, err)
	sugar.Infof("GardenHTLC deployment transaction sent: %s", tx.Hash().Hex())

	_, err = bind.WaitMined(context.Background(), client, tx)
	require.NoError(t, err)
	sugar.Infof("GardenHTLC contract deployed at: %s", htlcAddress.Hex())

	return htlcAddress, htlc
}

func approveTokens(t *testing.T, client *ethclient.Client, transactor *bind.TransactOpts, token *bindings.TestERC20, htlcAddress common.Address, sugar *zap.SugaredLogger) {
	sugar.Info("Approving tokens for HTLC contract")
	tx, err := token.Approve(transactor, htlcAddress, big.NewInt(1000000))
	require.NoError(t, err)
	sugar.Infof("Approval transaction sent: %s", tx.Hash().Hex())

	_, err = bind.WaitMined(context.Background(), client, tx)
	require.NoError(t, err)
	sugar.Info("Token approval completed")
}

func setupWatcher(t *testing.T, client *ethclient.Client, htlcAddress common.Address, logger *zap.Logger) *watcher.Watcher {
	contractConfig, err := htlcConfig.HTLCConfig(htlcAddress, client)
	require.NoError(t, err)

	config := watcher.WatcherConfig{
		EthClient:       client,
		Contracts:       []watcher.ContractConfig{contractConfig},
		MaxBlockSpan:    1000,
		PollingInterval: 100 * time.Millisecond,
		Logger:          logger,
		WorkerCount:     10,
		QueueSize:       100,
		StartBlock:      0,
	}

	w, err := watcher.NewWatcher(config)
	require.NoError(t, err)
	return w
}

func testInitiateAndRedeem(t *testing.T, client *ethclient.Client, transactor *bind.TransactOpts, htlc *gardenhtlc.GardenHTLC, aliceAddress, bobAddress common.Address, sugar *zap.SugaredLogger, db *store.Store) {
	secret := randomSecret()
	secretHash := sha256.Sum256(secret)
	timelock := big.NewInt(1)
	amount := big.NewInt(1000)

	swapID := sha256.Sum256(append(secretHash[:], common.BytesToHash(aliceAddress.Bytes()).Bytes()...))
	createSwap(t, db, hex.EncodeToString(swapID[:]), amount, secretHash[:])

	sugar.Info("Initiating first HTLC")
	tx, err := htlc.Initiate(transactor, bobAddress, timelock, amount, secretHash)
	require.NoError(t, err)
	sugar.Infof("Initiate transaction sent: %s", tx.Hash().Hex())

	receipt, err := bind.WaitMined(context.Background(), client, tx)
	require.NoError(t, err)
	sugar.Info("Initiate transaction mined")

	orderID := receipt.Logs[0].Topics[1]
	sugar.Infof("HTLC initiated with orderID: %s", orderID.Hex())

	sugar.Info("Redeeming HTLC")
	tx, err = htlc.Redeem(transactor, orderID, secret)
	require.NoError(t, err)
	sugar.Infof("Redeem transaction sent: %s", tx.Hash().Hex())

	_, err = bind.WaitMined(context.Background(), client, tx)
	require.NoError(t, err)
	sugar.Info("Redeem transaction mined")
}

func testInitiateAndRefund(t *testing.T, client *ethclient.Client, transactor *bind.TransactOpts, htlc *gardenhtlc.GardenHTLC, aliceAddress, bobAddress common.Address, sugar *zap.SugaredLogger, db *store.Store) {
	secret := randomSecret()
	secretHash := sha256.Sum256(secret)
	timelock := big.NewInt(1)
	amount := big.NewInt(1000)

	swapID := sha256.Sum256(append(secretHash[:], common.BytesToHash(aliceAddress.Bytes()).Bytes()...))
	createSwap(t, db, hex.EncodeToString(swapID[:]), amount, secretHash[:])

	sugar.Info("Initiating second HTLC (for refund)")
	tx, err := htlc.Initiate(transactor, bobAddress, timelock, amount, secretHash)
	require.NoError(t, err)
	sugar.Infof("Second initiate transaction sent: %s", tx.Hash().Hex())

	receipt, err := bind.WaitMined(context.Background(), client, tx)
	require.NoError(t, err)
	sugar.Info("Second initiate transaction mined")

	refundOrderID := receipt.Logs[0].Topics[1]
	sugar.Infof("Second HTLC initiated with orderID: %s", refundOrderID.Hex())

	// Mine a new block
	err = client.Client().Call(nil, "evm_mine")
	require.NoError(t, err)

	sugar.Info("Refunding HTLC")
	tx, err = htlc.Refund(transactor, refundOrderID)
	require.NoError(t, err)
	sugar.Infof("Refund transaction sent: %s", tx.Hash().Hex())

	_, err = bind.WaitMined(context.Background(), client, tx)
	require.NoError(t, err)
	sugar.Info("Refund transaction mined")
}

func createSwap(t *testing.T, db *store.Store, swapID string, amount *big.Int, secretHash []byte) {
	swap := &model.Swap{
		SwapID:       swapID,
		Amount:       model.BigInt{Int: amount},
		Chain:        "ETH",
		Asset:        "ETH",
		Initiator:    "0x123",
		Redeemer:     "0x456",
		TimeLock:     model.BigInt{Int: big.NewInt(1)},
		SecretHash:   secretHash,
		FilledAmount: model.BigInt{Int: big.NewInt(0)},
	}

	err := db.CreateSwap(swap)
	require.NoError(t, err)
}

func randomSecret() []byte {
	secret := [32]byte{}
	_, err := rand.Read(secret[:])
	if err != nil {
		panic(err)
	}
	return secret[:]
}
