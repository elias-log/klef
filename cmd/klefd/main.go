// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package main

import (
	"context"
	"fmt"
	"klef/config"
	"klef/core"
	"klef/replica"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := config.LoadConfig()

	signer, err := core.NewEd25519Signer()
	if err != nil {
		panic(err)
	}

	r := replica.NewReplica(cfg, signer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := r.Start(ctx); err != nil {
		panic(err)
	}

	fmt.Printf(">>> [RUN] Klef Replica %d...\n", cfg.NodeID)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh

	fmt.Println(">>> [SHUTDOWN]")

	if err := r.Stop(); err != nil {
		panic(err)
	}
}
