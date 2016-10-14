package main

import (
	"encoding/binary"
	"fmt"

	. "github.com/tendermint/go-common"
	"github.com/tendermint/tmsp/types"
)

type CounterApplication struct {
	hashCount int
	txCount   int
}

func NewCounterApplication() *CounterApplication {
	return &CounterApplication{}
}

func (app *CounterApplication) Info() string {
	return Fmt("hashes:%v, txs:%v", app.hashCount, app.txCount)
}

func (app *CounterApplication) SetOption(key string, value string) (string) {
	return ""
}

func (app *CounterApplication) AppendTx(tx []byte) types.Result {
	app.txCount += 1

	hash := make([]byte, 8)
	binary.BigEndian.PutUint64(hash, uint64(app.txCount))

	fmt.Printf("AppendTx response %v data \n\n", append(hash, tx...))

	return types.NewResultOK(append(hash, tx...), "")
}

func (app *CounterApplication) CheckTx(tx []byte) types.Result {
	return types.OK
}

func (app *CounterApplication) Commit() types.Result {
	app.hashCount += 1

	if app.txCount == 0 {
		return types.OK
	} else {
		hash := make([]byte, 8)
		binary.BigEndian.PutUint64(hash, uint64(app.txCount))
		return types.NewResultOK(hash, "")
	}
}

func (app *CounterApplication) Query(query []byte) types.Result {
	return types.NewResultOK(nil, "Query is not supported")
}
