package watcher

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

// EventHandler is a function type that handles an event
type EventHandler func(ctx context.Context, event types.Log) error

// ContractConfig represents the configuration for a single contract to watch
type ContractConfig struct {
	Address       common.Address
	EventHandlers map[common.Hash]EventHandler
}

func NewContractConfig(address common.Address, contractABI abi.ABI, handlers map[string]EventHandler) (ContractConfig, error) {
	config := ContractConfig{
		Address:       address,
		EventHandlers: make(map[common.Hash]EventHandler),
	}

	for eventName, handler := range handlers {
		event, exists := contractABI.Events[eventName]
		if !exists {
			return ContractConfig{}, fmt.Errorf("event %s not found in ABI", eventName)
		}
		config.EventHandlers[event.ID] = handler
	}

	return config, nil
}

// WatcherConfig represents the configuration for the watcher service
type WatcherConfig struct {
	EthClient       *ethclient.Client
	Contracts       []ContractConfig
	MaxBlockSpan    uint64
	PollingInterval time.Duration
	Logger          *zap.Logger
	StartBlock      uint64
	WorkerCount     int
	QueueSize       int
}

// EventJob represents a job for the worker pool
type EventJob struct {
	Event   types.Log
	Handler EventHandler
}

type Watcher struct {
	config            WatcherConfig
	currentBlock      uint64
	quit              chan struct{}
	mu                *sync.Mutex
	jobQueue          chan EventJob
	watcherWg         *sync.WaitGroup
	workerWg          *sync.WaitGroup
	addresses         []common.Address
	topics            []common.Hash
	addressToHandlers map[common.Address]map[common.Hash]EventHandler
}

// NewWatcher creates a new Watcher instance
func NewWatcher(config WatcherConfig) (*Watcher, error) {
	if config.EthClient == nil {
		return nil, fmt.Errorf("EthClient is required")
	}
	if config.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if config.WorkerCount <= 0 {
		return nil, fmt.Errorf("WorkerCount must be greater than 0")
	}
	if config.QueueSize <= 0 {
		return nil, fmt.Errorf("QueueSize must be greater than 0")
	}

	w := &Watcher{
		config:            config,
		mu:                new(sync.Mutex),
		jobQueue:          make(chan EventJob, config.QueueSize),
		workerWg:          new(sync.WaitGroup),
		quit:              make(chan struct{}),
		watcherWg:         new(sync.WaitGroup),
		addressToHandlers: make(map[common.Address]map[common.Hash]EventHandler),
	}

	// Initialize addresses, topics, and addressToHandlers
	topicsMap := make(map[common.Hash]bool)
	for _, contract := range config.Contracts {
		w.addresses = append(w.addresses, contract.Address)
		w.addressToHandlers[contract.Address] = contract.EventHandlers
		for eventID := range contract.EventHandlers {
			if !topicsMap[eventID] {
				w.topics = append(w.topics, eventID)
				topicsMap[eventID] = true
			}
		}
	}

	return w, nil
}

// Start begins the watching process
func (w *Watcher) Start(ctx context.Context) error {
	w.config.Logger.Info("Starting watcher service")

	// Start worker pool
	for i := 0; i < w.config.WorkerCount; i++ {
		w.workerWg.Add(1)
		go w.worker(ctx)
	}

	w.currentBlock = w.config.StartBlock

	// Start the watcher
	w.watcherWg.Add(1)
	w.start(ctx)

	return nil
}

func (w *Watcher) start(ctx context.Context) {
	ticker := time.NewTicker(w.config.PollingInterval)
	go func() {
		defer ticker.Stop()
		defer w.watcherWg.Done()
		for {
			select {
			case <-ctx.Done():
				close(w.jobQueue)
				w.workerWg.Wait()
				w.config.Logger.Info("Watcher service stopped")
				return
			case <-ticker.C:
				if err := w.pollEvents(ctx); err != nil {
					if err == context.Canceled {
						return
					}
					w.config.Logger.Error("Error polling events", zap.Error(err))
				}
			case <-w.quit:
				return
			}
		}
	}()
}

func (w *Watcher) Stop() error {
	if w.quit == nil {
		return fmt.Errorf("watcher already stopped")
	}

	w.config.Logger.Info("stopping watcher")
	close(w.quit)

	w.config.Logger.Info("waiting for watcher to stop")
	w.watcherWg.Wait()

	w.quit = nil
	return nil
}

func (w *Watcher) worker(ctx context.Context) {
	defer w.workerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-w.jobQueue:
			if !ok {
				return
			}
			if err := job.Handler(ctx, job.Event); err != nil {
				if err == context.Canceled {
					return
				}
				w.config.Logger.Error("Error handling event", zap.Error(err), zap.String("eventID", job.Event.Topics[0].Hex()))
			}
		}
	}
}

func (w *Watcher) pollEvents(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	latestBlock, err := w.config.EthClient.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest block number: %w", err)
	}

	if latestBlock <= w.currentBlock {
		return nil // No new blocks to process
	}

	fromBlock := w.currentBlock + 1
	toBlock := latestBlock

	if toBlock-fromBlock > w.config.MaxBlockSpan {
		toBlock = fromBlock + w.config.MaxBlockSpan
	}

	w.config.Logger.Info("Polling events", zap.Uint64("fromBlock", fromBlock), zap.Uint64("toBlock", toBlock))

	if err := w.processAllContractEvents(ctx, fromBlock, toBlock); err != nil {
		w.config.Logger.Error("Error processing contract events", zap.Error(err))
		return err
	}

	w.currentBlock = toBlock
	return nil
}

func (w *Watcher) processAllContractEvents(ctx context.Context, fromBlock, toBlock uint64) error {
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(fromBlock)),
		ToBlock:   big.NewInt(int64(toBlock)),
		Addresses: w.addresses,
		Topics:    [][]common.Hash{w.topics},
	}

	logs, err := w.config.EthClient.FilterLogs(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to filter logs: %w", err)
	}

	w.config.Logger.Info("Found logs", zap.Int("count", len(logs)))

	for _, log := range logs {
		if handlers, ok := w.addressToHandlers[log.Address]; ok {
			if handler, ok := handlers[log.Topics[0]]; ok {
				select {
				case w.jobQueue <- EventJob{Event: log, Handler: handler}:
				case <-ctx.Done():
					return ctx.Err()
				default:
					w.config.Logger.Warn("Job queue is full, dropping event",
						zap.String("contractAddress", log.Address.Hex()),
						zap.String("eventID", log.Topics[0].Hex()))
				}
			}
		}
	}

	return nil
}
