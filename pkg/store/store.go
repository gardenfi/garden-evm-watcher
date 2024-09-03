package store

import (
	"context"
	"errors"
	"math/big"

	"github.com/gardenfi/garden-evm-watcher/pkg/model"
	"gorm.io/gorm"
)

type contextKey struct{}

var storeKey = contextKey{}

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	err := db.AutoMigrate(
		&model.CreateOrder{},
		&model.FillOrder{},
		&model.Swap{},
		&model.MatchedOrder{},
		&model.Strategy{},
	)
	if err != nil {
		panic(err)
	}
	return &Store{db: db}
}

func StoreFromContext(ctx context.Context) (*Store, error) {
	store, ok := ctx.Value(storeKey).(*Store)
	if !ok {
		return nil, errors.New("store not found in context")
	}
	return store, nil

}

func ContextWithStore(ctx context.Context, store *Store) context.Context {
	return context.WithValue(ctx, storeKey, store)
}

func (s *Store) CreateSwap(swap *model.Swap) error {
	return s.db.Create(swap).Error
}

func (s *Store) UpdateSwapInitiate(orderID string, initiateTxHash string, filledAmount, initiateBlockNumber *big.Int) error {
	result := s.db.Model(&model.Swap{}).
		Where("swap_id = ?", orderID).
		Updates(map[string]interface{}{
			"initiate_tx_hash":      initiateTxHash,
			"filled_amount":         filledAmount,
			"initiate_block_number": model.BigInt{Int: initiateBlockNumber},
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("no swap found with the given orderID")
	}

	return nil
}

func (s *Store) UpdateSwapRedeem(orderID string, redeemTxHash string, secret []byte, redeemBlockNumber *big.Int) error {
	result := s.db.Model(&model.Swap{}).
		Where("swap_id = ?", orderID).
		Updates(map[string]interface{}{
			"redeem_tx_hash":      redeemTxHash,
			"secret":              secret,
			"redeem_block_number": model.BigInt{Int: redeemBlockNumber},
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("no swap found with the given orderID")
	}

	return nil
}

func (s *Store) UpdateSwapRefund(orderID string, refundTxHash string, refundBlockNumber *big.Int) error {
	result := s.db.Model(&model.Swap{}).
		Where("swap_id = ?", orderID).
		Updates(map[string]interface{}{
			"refund_tx_hash":      refundTxHash,
			"refund_block_number": model.BigInt{Int: refundBlockNumber},
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("no swap found with the given orderID")
	}

	return nil
}

func (s *Store) WriteCreateOrder(order *model.CreateOrder) error {
	result := s.db.Create(order)
	return result.Error
}

func (s *Store) ReadCreateOrder(createId string) (*model.CreateOrder, error) {
	order := &model.CreateOrder{}
	result := s.db.Where("create_id = ?", createId).First(order)
	if result.Error != nil {
		return nil, result.Error
	}
	return order, nil
}
