package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"gorm.io/gorm"
)

type Strategy struct {
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
	Filler             string         `gorm:"type:text;not null"`
	SourceChainAddress string         `gorm:"type:text;not null"`
	DestChainAddress   string         `gorm:"type:text;not null"`
	SourceChain        string         `gorm:"type:text;not null"`
	DestChain          string         `gorm:"type:text;not null"`
	SourceAsset        string         `gorm:"type:text;not null"`       // whitelisted source assets, nil means allowing any asset
	DestAsset          string         `gorm:"type:text;not null"`       // whitelisted destination assets, nil means allowing any asset
	Makers             []string       `gorm:"type:text[]"`              // whitelisted makers, nil means allowing any maker
	MinAmount          BigInt         `gorm:"type:decimal;not null"`    // minimum amount
	MaxAmount          BigInt         `gorm:"type:decimal" default:"0"` // maximum amount
	MinSourceTimeLock  uint64         `gorm:"type:integer;not null"`    // minimum time lock
	MinPrice           float64        `gorm:"type:real;not null"`       // minimum price
	Fee                uint64         `gorm:"type:integer;not null"`    // fee in basis points
}

type CreateOrder struct {
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
	DeletedAt                   gorm.DeletedAt `gorm:"index"`
	CreateID                    string         `gorm:"type:text;primaryKey;not null"`
	BlockNumber                 BigInt         `gorm:"type:decimal;not null"`
	SourceChain                 string         `gorm:"type:text;not null"`
	DestinationChain            string         `gorm:"type:text;not null"`
	SourceAsset                 string         `gorm:"type:text;not null"`
	DestinationAsset            string         `gorm:"type:text;not null"`
	InitiatorSourceAddress      string         `gorm:"type:text;not null"`
	InitiatorDestinationAddress string         `gorm:"type:text;not null"`
	SourceAmount                BigInt         `gorm:"type:decimal;not null"`
	DestinationAmount           BigInt         `gorm:"type:decimal;not null"`
	Fee                         BigInt         `gorm:"type:decimal;not null"`
	Nonce                       BigInt         `gorm:"type:decimal;not null"`
	MinDestinationConfirmations uint64         `gorm:"type:integer;not null"`
	TimeLock                    uint64         `gorm:"type:integer;not null"`
	SecretHash                  []byte         `gorm:"type:bytea;not null"`
}

type FillOrder struct {
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	DeletedAt                  gorm.DeletedAt `gorm:"index"`
	FillID                     string         `gorm:"type:text;primaryKey;not null"`
	BlockTimestamp             BigInt         `gorm:"type:decimal;not null"`
	SourceChain                string         `gorm:"type:text;not null"`
	DestinationChain           string         `gorm:"type:text;not null"`
	SourceAsset                string         `gorm:"type:text;not null"`
	DestinationAsset           string         `gorm:"type:text;not null"`
	RedeemerSourceAddress      string         `gorm:"type:text;not null"`
	RedeemerDestinationAddress string         `gorm:"type:text;not null"`
	SourceAmount               BigInt         `gorm:"type:decimal;not null"`
	DestinationAmount          BigInt         `gorm:"type:decimal;not null"`
	Fee                        BigInt         `gorm:"type:decimal;not null"`
	Nonce                      BigInt         `gorm:"type:decimal;not null"`
	MinSourceConfirmations     uint64         `gorm:"type:integer;not null"`
	TimeLock                   uint64         `gorm:"type:integer;not null"`
}

type MatchedOrder struct {
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt `gorm:"index"`
	CreateOrderID     string         `gorm:"type:text;not null"`
	FillOrderID       string         `gorm:"type:text;not null"`
	SourceSwapID      string         `gorm:"type:text;not null"`
	DestinationSwapID string         `gorm:"type:text;not null"`
	SourceSwap        Swap           `gorm:"foreignKey:SourceSwapID;references:SwapID"`
	DestinationSwap   Swap           `gorm:"foreignKey:DestinationSwapID;references:SwapID"`
	CreateOrder       CreateOrder    `gorm:"foreignKey:CreateOrderID;references:CreateID"`
	FillOrder         FillOrder      `gorm:"foreignKey:FillOrderID;references:FillID"`
}

type Swap struct {
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           gorm.DeletedAt `gorm:"index"`
	SwapID              string         `gorm:"type:text;primaryKey;not null"`
	Chain               string         `gorm:"type:text;not null"`
	Asset               string         `gorm:"type:text;not null"`
	Initiator           string         `gorm:"type:text;not null"`
	Redeemer            string         `gorm:"type:text;not null"`
	TimeLock            BigInt         `gorm:"type:decimal;not null"`
	FilledAmount        BigInt         `gorm:"type:decimal;not null"`
	Amount              BigInt         `gorm:"type:decimal;not null"`
	SecretHash          []byte         `gorm:"type:bytea;not null"`
	Secret              []byte         `gorm:"type:bytea"`
	InitiateTxHash      string         `gorm:"type:text"`
	RedeemTxHash        string         `gorm:"type:text"`
	RefundTxHash        string         `gorm:"type:text"`
	InitiateBlockNumber BigInt         `gorm:"type:decimal" default:"0"`
	RedeemBlockNumber   BigInt         `gorm:"type:decimal" default:"0"`
	RefundBlockNumber   BigInt         `gorm:"type:decimal" default:"0"`
}

type BigInt struct {
	*big.Int
}

// Scan implements the sql.Scanner interface
func (bi *BigInt) Scan(value interface{}) error {
	if value == nil {
		bi.Int = new(big.Int).SetInt64(0)
		return nil
	}

	switch v := value.(type) {
	case int64:
		bi.Int = big.NewInt(v)
		return nil
	case []byte:
		bi.Int = new(big.Int).SetInt64(0)
		return bi.Int.UnmarshalText(v)
	case string:
		if v == "" {
			v = "0"
		}
		bi.Int = new(big.Int).SetInt64(0)
		return bi.Int.UnmarshalText([]byte(v))
	default:
		bi.Int = new(big.Int).SetInt64(0)
		return errors.New("unsupported type for BigInt")
	}
}

// Value implements the driver.Valuer interface
func (bi BigInt) Value() (driver.Value, error) {
	if bi.Int == nil {
		return new(big.Int).SetInt64(0), nil
	}
	return bi.Int.Text(10), nil
}

// MarshalJSON implements the json.Marshaler interface
func (bi BigInt) MarshalJSON() ([]byte, error) {
	if bi.Int == nil {
		return json.Marshal("0")
	}
	return json.Marshal(bi.Int.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (bi *BigInt) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	if str == "" {
		str = "0"
	}
	var ok bool
	bi.Int, ok = new(big.Int).SetString(str, 10)
	if !ok {
		return errors.New("invalid BigInt value")
	}
	return nil
}
