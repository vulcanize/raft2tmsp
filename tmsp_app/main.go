package main

import (
	"flag"

	. "github.com/tendermint/go-common"
	"github.com/tendermint/tmsp/server"
)

func main() {

	addrPtr := flag.String("addr", "tcp://0.0.0.0:46658", "Listen address")
	tmspPtr := flag.String("tmsp", "socket", "TMSP server: socket | grpc")
	flag.Parse()
	app := NewCounterApplication()

	// Start the listener
	_, err := server.NewServer(*addrPtr, *tmspPtr, app)
	if err != nil {
		Exit(err.Error())
	}

	// Wait forever
	TrapSignal(func() {
		// Cleanup
	})

}
