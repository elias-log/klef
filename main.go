package main

import (
	"arachnet-bft/config"
	"arachnet-bft/core"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := config.LoadConfig()
	signer, _ := core.NewEd25519Signer() // 에러 처리는 생략했네

	v := core.NewValidator(cfg.NodeID, cfg, signer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go v.Start(ctx)
	fmt.Printf(">>> [RUN] Arachnet-BFT Node %d 가동 중...\n", cfg.NodeID)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	fmt.Println(">>> [SHUTDOWN] 안전하게 종료하네.")
}
