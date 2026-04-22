// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Mempool acts as the primary ingress buffer for client-submitted transactions.

Design intent:
- Validate cryptographic signatures and structural integrity of transactions.
- Track object dependencies to enable parallel execution at the shard level.
- Surface potential conflicts early to aid proposer selection without enforcing resolution.

Note:
- Strict conflict resolution is intentionally deferred to the anchoring/finalization stage
  to maintain global consistency across nodes.
- Early resolution in the mempool is avoided to prevent non-deterministic state divergence.
- Will be integrated with VRF-based shard assignment logic (Phase 6).
- Serves as the first stage of the transaction lifecycle before DAG injection.
*/

package core
