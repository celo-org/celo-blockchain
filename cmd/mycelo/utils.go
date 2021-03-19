package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func exit(err interface{}) {
	if err == nil {
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// withExitSignals returns a context that will stop whenever
// the process receive a SIGINT / SIGTERM
func withExitSignals(parentCtx context.Context) context.Context {
	ctx, stop := context.WithCancel(parentCtx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press CTRL-C to stop the process")
	go func() {
		select {
		case <-parentCtx.Done():
		case sig := <-sigs:
			fmt.Printf("Got Signal, Shutting down... signal: %s", sig.String())
		}
		stop()
	}()
	return ctx
}
