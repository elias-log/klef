// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package replica

import (
	"context"
	"fmt"
)

/// Start initializes background maintenance loops and the ingress router.
func (r *Replica) Start(ctx context.Context) error {
	r.ctx, r.cancel = context.WithCancel(ctx)

	go r.pendingMgr.StartCleanupLoop(r.ctx)
	go r.runMessageLoop()

	return nil
}

/// runMessageLoop executes the primary ingress event loop for network messages.
func (r *Replica) runMessageLoop() {
	fmt.Printf("[SYSTEM] Replica %d: Message routing loop started.\n", r.id)

	for {
		select {
		case msg, ok := <-r.InboundMsg:
			if !ok {
				return
			}
			r.routeMessage(msg)

		case <-r.ctx.Done():
			fmt.Printf("[SYSTEM] Replica %d: Shutting down...\n", r.id)
			return
		}
	}
}

/// Stop terminates all background runtime processes.
func (r *Replica) Stop() error {
	if r.cancel != nil {
		r.cancel()
	}

	close(r.InboundMsg)
	return nil
}
