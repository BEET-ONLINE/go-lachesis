package evm_core

import (
	"github.com/Fantom-foundation/go-ethereum/core/types"
	
	"github.com/Fantom-foundation/go-lachesis/ethapi/common"
)

// NewTxsNotify is posted when a batch of transactions enter the transaction pool.
type NewTxsNotify struct{ Txs []*types.Transaction }

// PendingLogsNotify is posted pre mining and notifies of pending logs.
type PendingLogsNotify struct {
	Logs []*types.Log
}

// NewMinedBlockNotify is posted when a block has been imported.
type NewMinedBlockNotify struct{ Block *EvmBlock }

// RemovedLogsNotify is posted when a reorg happens
type RemovedLogsNotify struct{ Logs []*types.Log }

type ChainNotify struct {
	Block *EvmBlock
	Hash  common.Hash
	Logs  []*types.Log
}

type ChainSideNotify struct {
	Block *EvmBlock
}

type ChainHeadNotify struct{ Block *EvmBlock }
