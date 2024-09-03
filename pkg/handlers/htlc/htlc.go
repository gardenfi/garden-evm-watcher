package htlc

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/catalogfi/blockchain/evm/bindings/contracts/htlc/gardenhtlc"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gardenfi/garden-evm-watcher/pkg/store"
	"github.com/gardenfi/garden-evm-watcher/pkg/watcher"
)

var (
	gardenHtlcAbi         *abi.ABI
	gardenHtlcEventFilter *gardenhtlc.GardenHTLCFilterer
)

func HTLCConfig(addr common.Address, ethClient *ethclient.Client) (watcher.ContractConfig, error) {
	var err error
	gardenHtlcAbi, err = gardenhtlc.GardenHTLCMetaData.GetAbi()
	if err != nil {
		return watcher.ContractConfig{}, fmt.Errorf("failed to get ABI: %v", err)
	}

	gardenHtlcEventFilter, err = gardenhtlc.NewGardenHTLCFilterer(addr, ethClient)
	if err != nil {
		return watcher.ContractConfig{}, fmt.Errorf("failed to instantiate filterer: %v", err)
	}

	instance, err := gardenhtlc.NewGardenHTLC(addr, ethClient)
	if err != nil {
		return watcher.ContractConfig{}, fmt.Errorf("failed to instantiate contract: %v", err)
	}
	_, err = instance.Token(&bind.CallOpts{})
	if err != nil {
		return watcher.ContractConfig{}, fmt.Errorf("failed to call Token function, possibly not a GardenHTLC contract: %v", err)
	}

	return watcher.ContractConfig{
		Address:       addr,
		EventHandlers: constructHandlers(),
	}, nil
}

func constructHandlers() map[common.Hash]watcher.EventHandler {
	eventHandlers := make(map[common.Hash]watcher.EventHandler)

	eventHandlers[gardenHtlcAbi.Events["Initiated"].ID] = handleInitiatedEvent
	eventHandlers[gardenHtlcAbi.Events["Redeemed"].ID] = handleRedeemedEvent
	eventHandlers[gardenHtlcAbi.Events["Refunded"].ID] = handleRefundedEvent

	return eventHandlers
}

func handleInitiatedEvent(ctx context.Context, log types.Log) error {
	db, err := store.StoreFromContext(ctx)
	if err != nil {
		return err
	}
	event, err := gardenHtlcEventFilter.ParseInitiated(log)
	if err != nil {
		return err
	}
	return db.UpdateSwapInitiate(
		hex.EncodeToString(event.OrderID[:]),
		event.Raw.TxHash.String(),
		event.Amount,
		new(big.Int).SetUint64(event.Raw.BlockNumber),
	)
}

func handleRedeemedEvent(ctx context.Context, log types.Log) error {
	db, err := store.StoreFromContext(ctx)
	if err != nil {
		return err
	}
	event, err := gardenHtlcEventFilter.ParseRedeemed(log)
	if err != nil {
		return err
	}
	return db.UpdateSwapRedeem(
		hex.EncodeToString(event.OrderID[:]),
		event.Raw.TxHash.String(),
		event.Secret,
		new(big.Int).SetUint64(event.Raw.BlockNumber),
	)
}

func handleRefundedEvent(ctx context.Context, log types.Log) error {
	db, err := store.StoreFromContext(ctx)
	if err != nil {
		return err
	}
	event, err := gardenHtlcEventFilter.ParseRefunded(log)
	if err != nil {
		return err
	}
	return db.UpdateSwapRefund(
		hex.EncodeToString(event.OrderID[:]),
		event.Raw.TxHash.String(),
		new(big.Int).SetUint64(event.Raw.BlockNumber),
	)
}
