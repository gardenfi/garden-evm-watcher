package builder

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/catalogfi/garden-evm-watcher/cmd/config"
	"github.com/catalogfi/garden-evm-watcher/pkg/handlers/htlc"
	"github.com/catalogfi/garden-evm-watcher/pkg/handlers/orderbook"
	"github.com/catalogfi/garden-evm-watcher/pkg/store"
	"github.com/catalogfi/garden-evm-watcher/pkg/watcher"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func RunWatcher(cfg config.Config) {
	logger, err :=
		zap.NewProduction()
	if err != nil {
		panic(err)
	}

	watcherCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := gorm.Open(postgres.Open(cfg.PsqlDb), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	pgbd := store.NewStore(db)

	ctx := store.ContextWithStore(watcherCtx, pgbd)

	watchers, err := setupWatchers(cfg, logger)
	if err != nil {
		logger.Error("Failed to setup watchers", zap.Error(err))
	}

	for _, w := range watchers {
		go func(w *watcher.Watcher) {
			err := w.Start(ctx)
			if err != nil {
				logger.Error("Failed to start watcher", zap.Error(err))
			}
		}(w)
	}

	// waiting for system interrupt
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGKILL)
	<-sigs
}

func setupWatchers(cfg config.Config, logger *zap.Logger) ([]*watcher.Watcher, error) {
	getConfig := func(logger *zap.Logger, chainConfig config.ChainConfig, client *ethclient.Client) (watcher.WatcherConfig, error) {
		handlers := []watcher.ContractConfig{}
		var err error
		for contractAddr, abiType := range chainConfig.Contracts {
			logger.Info("Starting watcher for contract", zap.String("contract", contractAddr))

			contractHandler := watcher.ContractConfig{}
			switch abiType {

			case config.Orderbook:
				{
					contractHandler, err = orderbook.OrderbookConfig(common.HexToAddress(contractAddr), client)
					if err != nil {
						logger.Error("Failed to setup orderbook config", zap.Error(err))
						return watcher.WatcherConfig{}, err
					}
				}

			case config.GardenHTLC:
				{
					contractHandler, err = htlc.HTLCConfig(common.HexToAddress(contractAddr), client)
					if err != nil {
						logger.Error("Failed to setup htlc config", zap.Error(err))
						return watcher.WatcherConfig{}, err
					}
				}
			}
			handlers = append(handlers, contractHandler)
		}
		return watcher.WatcherConfig{
			EthClient:       client,
			Contracts:       handlers,
			MaxBlockSpan:    chainConfig.MaxBlockSpan,
			PollingInterval: time.Duration(chainConfig.PollingInterval) * time.Millisecond,
			Logger:          logger,
			StartBlock:      chainConfig.StartBlock,
			WorkerCount:     10,
			QueueSize:       1000,
		}, nil
	}

	watchers := []*watcher.Watcher{}
	for chainName, chainConfig := range cfg.Chains {
		chainLogger := logger.With(zap.String("chain", chainName))
		chainLogger.Info("Starting watcher for chain", zap.String("chain", chainName))

		client, err := ethclient.Dial(chainConfig.Url)
		if err != nil {
			chainLogger.Error("Failed to connect to rpc", zap.Error(err))
			return nil, err
		}

		watcherConfig, err := getConfig(chainLogger, chainConfig, client)
		if err != nil {
			chainLogger.Error("Failed to get watcher config", zap.Error(err))
			return nil, err
		}

		w, err := watcher.NewWatcher(watcherConfig)
		if err != nil {
			chainLogger.Error("Failed to create watcher", zap.Error(err))
			return nil, err
		}

		watchers = append(watchers, w)
	}
	return watchers, nil
}
