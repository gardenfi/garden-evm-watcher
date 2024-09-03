package main

import (
	"github.com/gardenfi/garden-evm-watcher/cmd/builder"
	"github.com/gardenfi/garden-evm-watcher/cmd/config"
)

func main() {
	cfg := config.LoadConfiguration("local_config.json")
	builder.RunWatcher(cfg)
}
