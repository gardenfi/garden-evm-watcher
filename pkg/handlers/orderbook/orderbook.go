package orderbook

import (
	"context"
	"fmt"
	"math/big"

	"github.com/catalogfi/blockchain/evm"
	"github.com/catalogfi/blockchain/evm/bindings/contracts/htlc/gardenhtlc"
	"github.com/catalogfi/garden-evm-watcher/pkg/model"
	"github.com/catalogfi/garden-evm-watcher/pkg/store"
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
	db, err := store.StoreFromContext(ctx)
	if err != nil {
		return err
	}
	createorder, err := evm.UnpackCreateOrder(log.Data)
	if err != nil {
		return fmt.Errorf("failed to unpack create order event: %v", err)
	}

	return db.WriteCreateOrder(&model.CreateOrder{
		BlockNumber:                 model.BigInt{Int: new(big.Int).SetUint64(log.BlockNumber)},
		SourceChain:                 createorder.SourceChain,
		DestinationChain:            createorder.DestinationChain,
		SourceAsset:                 createorder.SourceAsset,
		DestinationAsset:            createorder.DestinationAsset,
		InitiatorSourceAddress:      createorder.InitiatorSourceAddress,
		InitiatorDestinationAddress: createorder.InitiatorDestinationAddress,
		SourceAmount:                model.BigInt{Int: createorder.SourceAmount},
		DestinationAmount:           model.BigInt{Int: createorder.DestinationAmount},
		Fee:                         model.BigInt{Int: createorder.Fee},
		Nonce:                       model.BigInt{Int: createorder.Nonce},
		MinDestinationConfirmations: createorder.MinDestinationConfirmations.Uint64(),
		TimeLock:                    createorder.TimeLock.Uint64(),
		SecretHash:                  createorder.SecretHash[:],
	})
}

func handleFilledEvent(ctx context.Context, log types.Log) error {
	fmt.Println("Received filled event")
	return nil
}
