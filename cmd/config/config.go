package config

import (
	"encoding/json"
	"os"
)

type AbiType string

var (
	GardenHTLC AbiType = "htlc"
	Orderbook  AbiType = "orderbook"
)

type ChainConfig struct {
	Contracts       map[string]AbiType `binding:"required"`
	Url             string             `binding:"required"`
	StartBlock      uint64
	MaxBlockSpan    uint64
	PollingInterval int64
}

type Config struct {
	PsqlDb string                 `binding:"required"`
	Chains map[string]ChainConfig `binding:"required"`
}

func LoadConfiguration(file string) Config {
	var config Config
	configFile, err := os.Open(file)
	if err != nil {
		panic(err)
	}
	defer configFile.Close()
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&config)
	return config
}
