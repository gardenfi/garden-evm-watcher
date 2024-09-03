package orderbook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/catalogfi/blockchain/evm"
	ob "github.com/catalogfi/blockchain/evm/bindings/contracts/orderbook"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gardenfi/garden-evm-watcher/pkg/model"
	"github.com/gardenfi/garden-evm-watcher/pkg/store"
	"github.com/gardenfi/garden-evm-watcher/pkg/watcher"
)

var (
	orderBookAbi *abi.ABI
)

func OrderbookConfig(addr common.Address, ethClient *ethclient.Client) (watcher.ContractConfig, error) {
	var err error
	orderBookAbi, err = ob.OrderbookMetaData.GetAbi()
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
	createOrder, err := evm.UnpackCreateOrder(log.Data[96:])
	if err != nil {
		return fmt.Errorf("failed to unpack create order event: %v", err)
	}

	cid := sha256.Sum256(append(log.TxHash[:], createOrder.SecretHash[:]...))

	return db.WriteCreateOrder(&model.CreateOrder{
		CreateID:                    hex.EncodeToString(cid[:]),
		BlockNumber:                 model.BigInt{Int: new(big.Int).SetUint64(log.BlockNumber)},
		SourceChain:                 createOrder.SourceChain,
		DestinationChain:            createOrder.DestinationChain,
		SourceAsset:                 createOrder.SourceAsset,
		DestinationAsset:            createOrder.DestinationAsset,
		InitiatorSourceAddress:      createOrder.InitiatorSourceAddress,
		InitiatorDestinationAddress: createOrder.InitiatorDestinationAddress,
		SourceAmount:                model.BigInt{Int: createOrder.SourceAmount},
		DestinationAmount:           model.BigInt{Int: createOrder.DestinationAmount},
		Fee:                         model.BigInt{Int: createOrder.Fee},
		Nonce:                       model.BigInt{Int: createOrder.Nonce},
		MinDestinationConfirmations: createOrder.MinDestinationConfirmations.Uint64(),
		TimeLock:                    createOrder.TimeLock.Uint64(),
		SecretHash:                  createOrder.SecretHash[:],
	})
}

func handleFilledEvent(ctx context.Context, log types.Log) error {
	fmt.Println("Received filled event")
	return nil
}
