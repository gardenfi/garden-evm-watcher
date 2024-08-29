package orderbook_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/catalogfi/blockchain/evm"
	"github.com/catalogfi/blockchain/evm/bindings/contracts/orderbook"
	orderbookConfig "github.com/catalogfi/garden-evm-watcher/pkg/handlers/orderbook"
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

	orderbookAddr, orderbook := deployOrderbook(t, client, transactor, sugar)

	w := setupWatcher(t, client, orderbookAddr, logger)

	watcherCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pgDB, cleanup := SetupTestDB(t)
	defer cleanup()

	db := store.NewStore(pgDB)

	ctx := store.ContextWithStore(watcherCtx, db)

	sugar.Info("Starting watcher")
	err := w.Start(ctx)
	assert.NoError(t, err)

	testOrderbook(t, orderbook, transactor, aliceAddress, bobAddress, sugar)

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

func deployOrderbook(t *testing.T, client *ethclient.Client, transactor *bind.TransactOpts, sugar *zap.SugaredLogger) (common.Address, *orderbook.Orderbook) {
	sugar.Info("Deploying TestERC20 token")
	orderbookAddr, tx, order, err := orderbook.DeployOrderbook(transactor, client)
	require.NoError(t, err)
	sugar.Infof("Orderbook deployment transaction sent: %s", tx.Hash().Hex())

	_, err = bind.WaitMined(context.Background(), client, tx)
	require.NoError(t, err)
	sugar.Infof("Orderbook deployed at: %s", orderbookAddr.Hex())

	return orderbookAddr, order
}

func testOrderbook(t *testing.T, orderbook *orderbook.Orderbook, transactor *bind.TransactOpts, aliceAddress, bobAddress common.Address, sugar *zap.SugaredLogger) {
	sugar.Info("Creating order")
	secret := randomSecret()
	createOrderTx, err := evm.PackCreateOrder(
		&evm.CreateOrder{
			SourceChain:                 "ETH",
			DestinationChain:            "ETH",
			SourceAsset:                 aliceAddress.Hex(),
			DestinationAsset:            bobAddress.Hex(),
			SourceAmount:                big.NewInt(1000000000000000000),
			DestinationAmount:           big.NewInt(1000000000000000000),
			Fee:                         big.NewInt(0),
			Nonce:                       big.NewInt(0),
			MinDestinationConfirmations: big.NewInt(0),
			TimeLock:                    big.NewInt(100),
			SecretHash:                  [32]byte(secret),
		},
	)
	require.NoError(t, err)

	tx, err :=
		orderbook.CreateOrder(transactor, createOrderTx)
	require.NoError(t, err)
	sugar.Infof("Create order transaction sent: %s", tx.Hash().Hex())
}

func setupWatcher(t *testing.T, client *ethclient.Client, orderbookAddr common.Address, logger *zap.Logger) *watcher.Watcher {
	contractConfig, err := orderbookConfig.OrderbookConfig(orderbookAddr, client)
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

func randomSecret() []byte {
	secret := [32]byte{}
	_, err := rand.Read(secret[:])
	if err != nil {
		panic(err)
	}
	return secret[:]
}
