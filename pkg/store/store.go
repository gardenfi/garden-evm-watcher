package store

import (
	"errors"
	"math/big"

	"github.com/catalogfi/garden-evm-watcher/pkg/model"
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) UpdateSwapInitiate(orderID string, initiateTxHash string, filledAmount, initiateBlockNumber *big.Int) error {
	result := s.db.Model(&model.Swap{}).
		Where("order_id = ?", orderID).
		Updates(map[string]interface{}{
			"initiate_tx_hash":      initiateTxHash,
			"filled_amount":         filledAmount,
			"initiate_block_number": initiateBlockNumber,
			"initiated_at":          gorm.Expr("CASE WHEN initiated_at = 0 THEN ? ELSE initiated_at END", initiateBlockNumber),
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("no swap found with the given orderID")
	}

	return nil
}

func (s *Store) UpdateSwapRedeem(orderID string, redeemTxHash string, secret []byte, redeemedAt, redeemBlockNumber *big.Int) error {
	result := s.db.Model(&model.Swap{}).
		Where("order_id = ?", orderID).
		Updates(map[string]interface{}{
			"redeem_tx_hash":      redeemTxHash,
			"secret":              secret,
			"redeemed_at":         redeemedAt,
			"redeem_block_number": redeemBlockNumber,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("no swap found with the given orderID")
	}

	return nil
}

func (s *Store) UpdateSwapRefund(orderID string, refundTxHash string, refundedAt, refundBlockNumber *big.Int) error {
	result := s.db.Model(&model.Swap{}).
		Where("order_id = ?", orderID).
		Updates(map[string]interface{}{
			"refund_tx_hash":      refundTxHash,
			"refunded_at":         refundedAt,
			"refund_block_number": refundBlockNumber,
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
