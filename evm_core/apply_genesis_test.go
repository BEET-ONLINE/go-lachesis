package evm_core

import (
	"testing"

	"github.com/Fantom-foundation/go-ethereum/core/rawdb"
	"github.com/stretchr/testify/assert"

	"github.com/Fantom-foundation/go-lachesis/kvdb/memorydb"
	"github.com/Fantom-foundation/go-lachesis/kvdb/no_key_is_err"
	"github.com/Fantom-foundation/go-lachesis/kvdb/table"
	"github.com/Fantom-foundation/go-lachesis/lachesis"
	"github.com/Fantom-foundation/go-lachesis/logger"
)

func TestApplyGenesis(t *testing.T) {
	assertar := assert.New(t)
	logger.SetTestMode(t)

	db := rawdb.NewDatabase(
		no_key_is_err.Wrap(
			table.New(
				memorydb.New(), []byte("evm_"))))

	// no genesis
	_, err := ApplyGenesis(db, nil)
	if !assertar.Error(err) {
		return
	}

	// the same genesis
	netA := lachesis.FakeNetConfig(3, 0)
	blockA1, err := ApplyGenesis(db, &netA)
	if !assertar.NoError(err) {
		return
	}
	blockA2, err := ApplyGenesis(db, &netA)
	if !assertar.NoError(err) {
		return
	}
	if !assertar.Equal(blockA1, blockA2) {
		return
	}

	// different genesis
	netB := lachesis.FakeNetConfig(4, 0)
	_, err = ApplyGenesis(db, &netB)
	if !assertar.Error(err) {
		return
	}

}
