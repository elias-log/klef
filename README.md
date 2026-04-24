This document outlines the current design direction and key ideas under active exploration.

# Klef
> Named after Klefki — a key bearer.
> State is modeled as versioned objects, where conflicts are resolved
> deterministically through explicit dependencies.

---

## 1. Summary

Klef is not a blockchain.

It is a **Byzantine-fault-tolerant distributed database**
built on a global DAG.

Instead of enforcing a single global order,
Klef guarantees:

> **Determinism of outcomes, not ordering in time.**

This is achieved by separating:

- **Data availability** (what exists)
- **Execution semantics** (what it means)

Transactions are not globally ordered.
They are:

- published as data (DataClaims)
- interpreted independently (ExecClaims)
- resolved deterministically later

Consensus is not used to decide *who is right*,
but only to ensure:

> **the data exists and is shared**

Conflicts are expected,
and are resolved **algorithmically**, not politically.

## 2. Full Design Overview

This project aims to address two fundamental limitations of existing blockchain systems. First, the reliance on a global state model enforces sequential execution, severely limiting parallelism. Second, systems that adopt optimistic execution for better user experience often sacrifice global consistency. To overcome these issues, this design introduces a hybrid architecture that combines an object-centric state model, a global data DAG (Data Availability & Ordering Layer), and a sharded BFT-based execution layer using Jolteon-style consensus.

State is represented not as a monolithic global store, but as a collection of independent objects, each identified by a unique ID and version. Transactions consume specific versions of objects and produce new ones, forming a versioned dependency graph. This structure allows fine-grained decomposition of state, enabling parallel execution of transactions that do not conflict. When dependencies exist, they are explicitly encoded in the DAG, forming the basis for validation and rollback decisions.

The data layer is implemented as a global DAG composed of immutable vertices. 

These vertices are of two types:
- DataClaims (DC), representing transaction availability
- ExecClaims (EC), representing execution results

Each is validated by distinct quorum certificates (DataQC and ExecQC). This DAG acts not merely as storage, but as a shared observable memory for the entire system. Crucially, the execution of transactions MUST NOT mutate existing vertices; instead, all execution artifacts—including state updates and validity claims—are recorded as separate, immutable DAG nodes. Each shard deterministically executes DataClaims over its assigned object set and produces ExecClaims as verifiable execution results. These claims are not globally final and may conflict. Final truth emerges only after deterministic resolution over the DAG. Other shards asynchronously observe and validate these results. Importantly, validation goes beyond signature verification: nodes must also ensure that referenced object versions are consistent with their local view, enforcing causal validity in addition to cryptographic validity.

The execution layer consists of sharded committees, each assigned transactions via VRF-based selection. Within each committee, a Jolteon-style BFT protocol is used to quickly produce quorum certificates (QC), providing optimistic confirmation. Users can rely on these QC-backed results for low-latency feedback, but they do not represent final global truth. Finality is achieved through a deterministic anchoring and topological ordering process applied to the global DAG.

A key principle of this system is that it guarantees determinism of outcomes rather than ordering in time. When multiple transactions compete for the same input objects, the system resolves conflicts using predefined deterministic rules (e.g., hash-based tie-breaking). Invalidated transactions are not isolated; their invalidity propagates through the dependency graph, ensuring consistent global resolution. The system thus operates not on isolated transactions, but on entire dependency structures.

The object-centric model introduces a potential state bloat problem, as fine-grained objects can accumulate rapidly. To address this, the system incorporates economic mechanisms such as storage deposits and deletion rebates, ensuring long-term sustainability of the state size.

Rollback is an inherent feature of the system, not a flaw. It is the cost of enabling optimistic execution prior to global consensus. However, rollbacks are fully deterministic and governed by the DAG and conflict resolution rules, ensuring that all nodes converge to the same final state. The system therefore provides two levels of finality: optimistic (QC-based) and global (anchoring-based).

Ultimately, this system is not designed to be the fastest blockchain in absolute terms. Instead, it is designed to scale with workload by exploiting transaction independence. It provides horizontal scalability, fault isolation, and partial liveness, ensuring that failures in one shard do not propagate across the entire network. Unlike traditional global-state systems, it aims to maintain performance even as system load increases.

---

### State Model

State is represented as independent versioned objects.

Transactions:
- consume specific object versions
- produce new versions

This forms a **dependency graph**, enabling:

- parallel execution of independent transactions
- explicit encoding of conflicts
- deterministic validation and rollback

---

### Data Layer (Global DAG)


The system is built on a global DAG composed of immutable vertices.

Two vertex types exist:

- **DataClaim (DC)** — *"What happened"*
  - transaction availability
  - validated by **DataQC**
  - contains payload + dependencies

- **ExecClaim (EC)** — *"What it means"*
  - execution results
  - validated by **ExecQC**
  - contains state transitions + validity assertions

This DAG is a **shared observable memory**, not just storage.

**Key rule:**
- vertices are immutable
- execution never mutates data
- all results are append-only

---

### Execution Model

Each shard:

- deterministically executes DataClaims
- over its assigned object set
- produces ExecClaims

ExecClaims:
- are not globally final
- may conflict
- are resolved later

> **Local vs Global Validity**
>
> Local committee consensus ensures that an execution claim is
> well-formed and agreed upon by a Byzantine-resilient quorum.
>
> However, it does not guarantee global correctness.
>
> Execution validity is determined only through
> deterministic resolution over the global DAG.

---

### Edge Semantics

Edges encode meaning:

- **DC → DC**: causal dependency
- **EC → DC**: execution interpretation

> ExecClaims are only valid relative to the DataClaims they reference.

**Constraints:**
- No explicit EC → EC edges
- Execution dependencies are implicit via DC structure

---

### Conflict Model

Multiple ExecClaims may interpret the same DataClaims differently.

This is expected.

Conflicts are **not resolved by consensus**.

They are resolved deterministically using:

- object version rules
- hash-based tie-breaking
- dependency propagation

---

### Finality Model

Two levels of finality:

- **Optimistic finality** (QC-based)
- **Global finality** (anchoring over DAG)

Finality emerges from **deterministic resolution**, not ordering.

---

### System Properties

- Horizontal scalability (object-level parallelism)
- Fault isolation across shards
- Partial liveness under failure
- Deterministic convergence

---

### Immutability Guarantee

Once published:

- DC and EC are immutable
- execution is append-only
- vertex hashes never change

This guarantees:

- verifiability
- data availability
- non-equivocation

---

### Tradeoffs

- Rollback is inherent (optimistic execution)
 
---

### State Growth

State size is controlled through economic mechanisms:

- storage fees
- deletion incentives

State reduction operates at the **object level**,
by removing or compacting unused object versions.

---

### History Pruning

The DAG stores historical data and execution artifacts.

- Pruning removes **historical vertices**, not live state
- Safe after anchoring and finalization
- Does not affect correctness

> Pruning operates on history, not on state.

---

### System Invariants

Klef relies on a small set of structural invariants:

- **Acyclic Dependencies**
  Dependency edges form a DAG, ensuring finite and deterministic execution and rollback

- **Single Invalidation**
  Each transaction (or ExecClaim) can be invalidated at most once

- **Immutability**
  All vertices (DC, EC) are append-only and never modified after publication

- **Deterministic Resolution**
  Given the same DAG, all nodes converge to the same final state

- **State–History Separation**
  State is derived from the DAG but managed independently;
  pruning historical data does not affect correctness

---
## 3. Implementation Waypoints

This project cannot be implemented in a single step. Instead, it must evolve incrementally under the principle of introducing only one new source of complexity at a time. The ultimate goal is a system combining a data DAG with a sharded Jolteon-based consensus layer, but the initial focus must be on building a deterministic state machine without consensus or sharding.

### Phase 0: DAG Structure and Synchronization (Done)
The goal is to construct a DAG with parent references and ensure it can be shared and reconstructed across nodes. At this stage, transactions carry no semantic meaning. Orphan handling must be implemented, buffering nodes whose parents have not yet arrived. The result is a consistent DAG structure across nodes given the same inputs.

### Phase 1: Single-node State Machine (Now)
Introduce the object model and transaction execution logic. Each object has an ID and version, and transactions consume specific versions to produce new ones. Executing transactions in canonical order must always yield the same state. The result is a fully deterministic execution environment on a single node.

### Phase 2: Deterministic Conflict Resolution (Tie-breaking)
Define rules for resolving conflicts when multiple transactions attempt to consume the same object version. For example, selecting the transaction with the lowest hash. This rule becomes the foundation for consistency across nodes. The result is that identical inputs always lead to identical outcomes, regardless of execution order.

### Phase 3: Multi-node Deterministic Replay
Extend the system to multiple nodes. Even if transactions are received in different orders, all nodes must converge to the same state using the DAG and conflict resolution rules. Gossip-based propagation and DAG merging are introduced. The key property validated here is that outcomes are rule-driven, not order-driven.

### Phase 4: Dependency Tracking and Rollback Propagation
Track dependencies between transactions via the DAG. When a transaction is invalidated, all dependent transactions must also be invalidated. The system must ensure deterministic rollback propagation across all nodes.

### Phase 5: Single-committee Consensus (Jolteon-lite)
Introduce a basic consensus mechanism by treating the entire network as a single committee. Implement leader proposal, voting, and QC generation. QC acts as optimistic confirmation and is attached to DAG entries.

### Phase 6: Sharding (No Cross-shard Transactions)
Partition the network into multiple shards, each executing transactions independently. Cross-shard transactions are not yet allowed. The goal is to validate scalability and stability as the number of shards increases.

### Phase 7: Cross-shard Interaction via DAG Observation
Enable shards to observe and validate the outputs of other shards via the global DAG. Execution remains local, but validation incorporates global information. Dependency graphs now span across shards.

### Phase 8: Anchoring and Global Ordering
Define a deterministic anchoring mechanism to finalize the global state. A snapshot of the DAG is taken, transactions are topologically sorted, and conflicts are resolved globally. The result is a consistent final state across all nodes.

---

## 4. Practical Deployment

Klef is designed not as a single-purpose system,
but as a **generalized distributed execution engine**.

Its key property is that **trust assumptions are configurable**
without changing the core architecture.

---

### Configurable Trust Model

Klef exposes consensus thresholds as parameters:

- **DataQC (global availability)**
- **ExecQC (local execution agreement)**

By adjusting these thresholds, the system can operate under
different environments:

- Public networks (Byzantine-resistant)
- Consortium systems (partially trusted)
- Enterprise deployments (trusted infrastructure)

This allows the same system to adapt across a wide spectrum
of trust models.

---

### Enterprise Optimization

In enterprise environments:

- participants are known and controlled
- strong Byzantine assumptions are often unnecessary

Klef can leverage this by:

- lowering consensus thresholds
- reducing coordination overhead
- increasing throughput and lowering latency

This transformation requires **no architectural changes** —
only parameter adjustment.

---

### Key Insight

Klef separates:

- **data availability (consensus)**
- **execution correctness (deterministic resolution)**

Because correctness is not decided by consensus:

- reducing consensus cost does not break correctness
- it only changes the cost and speed of coordination

---

### Result

This enables Klef to function as:

> a high-performance distributed database in trusted environments
> and a Byzantine-resilient system in adversarial environments

— using the same core design.

---

## License

This is a learning-focused project. The licensing structure is applied to both follow open-source practices and protect the original design.

All commits prior to April 22, 2026 should be considered unpublished work with no license granted.
The project is licensed under GPL v3 starting from April 22, 2026.

- **Code**: Licensed under GNU General Public License v3.0 or later.
- **Documentation**: All content within this repository including README.md is licensed under CC BY-NC-ND 4.0. (https://creativecommons.org/licenses/by-nc-nd/4.0/)

Copyright (c) 2026 elias-log.