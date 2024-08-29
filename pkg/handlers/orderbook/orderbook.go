package orderbook

import (
	"context"
	"fmt"

	"github.com/catalogfi/blockchain/evm/bindings/contracts/htlc/gardenhtlc"
	"github.com/catalogfi/garden-evm-watcher/pkg/watcher"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	orderBookAbi *abi.ABI
)

func OrderbookConfig(addr common.Address, ethClient *ethclient.Client) (watcher.ContractConfig, error) {
	var err error
	orderBookAbi, err = gardenhtlc.GardenHTLCMetaData.GetAbi()
	if err != nil {
		return watcher.ContractConfig{}, fmt.Errorf("failed to get ABI: %v", err)
	}

	return watcher.ContractConfig{
		Address:       addr,
		EventHandlers: constructHandlers(),
	}, nil
}

func constructHandlers() map[common.Hash]watcher.EventHandler {
	eventHandlers := make(map[common.Hash]watcher.EventHandler)

	eventHandlers[orderBookAbi.Events["Created"].ID] = handleCreatedEvent
	eventHandlers[orderBookAbi.Events["Filled"].ID] = handleFilledEvent

	return eventHandlers
}

func handleCreatedEvent(ctx context.Context, log types.Log) error {
	event, err := orderBookAbi.Events["Created"].Inputs.UnpackValues(log.Data)
	if err != nil {
		return fmt.Errorf("failed to unpack event data: %v", err)
	}

	fmt.Printf("Created event: %v\n", event)

	return nil
}

func handleFilledEvent(ctx context.Context, log types.Log) error {
	event, err := orderBookAbi.Events["Filled"].Inputs.UnpackValues(log.Data)
	if err != nil {
		return fmt.Errorf("failed to unpack event data: %v", err)
	}

	fmt.Printf("Filled event: %v\n", event)

	return nil
}
