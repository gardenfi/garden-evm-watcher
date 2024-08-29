package store

import (
	"context"
	"errors"
	"math/big"

	"github.com/catalogfi/garden-evm-watcher/pkg/model"
	"gorm.io/gorm"
)

type contextKey struct{}

var storeKey = contextKey{}

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
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

func (s *Store) UpdateSwapInitiate(orderID string, initiateTxHash string, filledAmount, initiateBlockNumber *big.Int) error {
	result := s.db.Model(&model.Swap{}).
		Where("order_id = ?", orderID).
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
		Where("order_id = ?", orderID).
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
		Where("order_id = ?", orderID).
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

func (s *Store) CreateCreateOrder(order *model.CreateOrder) error {
	result := s.db.Create(order)
	return result.Error
}
