# ApexLedger ⚡

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![OpenAPI](https://img.shields.io/badge/OpenAPI-3.1-6BA539?style=flat&logo=openapi-initiative)](https://openapis.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **Production-grade, spec-driven core banking ledger and money-movement engine built in Go.**
> Engineered to demonstrate financial infrastructure fundamentals: strict double-entry invariants ($\sum \text{debits} - \sum \text{credits} = 0$), deterministic concurrency controls, IETF `Idempotency-Key` guarantees, provider-agnostic banking rail adapters (BaaS), and transactional outbox patterns.

---

## 🏛️ System Architecture

```mermaid
flowchart TD
    subgraph ClientLayer [Client & Partner Integration]
        Client[API Consumers / Webhooks]
    end

    subgraph APILayer [API Gateway & Spec Enforcement]
        OpenAPI[OpenAPI 3.1 Spec] -.->|oapi-codegen| Router[Strict Generated Router]
        Idemp[Idempotency Layer<br/>IETF Draft Spec] --> Router
    end

    subgraph CoreEngine [Ledger & Transaction Engine]
        Router --> Service[Ledger Domain Service]
        Service --> Invariant[Double-Entry Invariant Engine<br/>Debits == Credits]
        Service --> StateMachine[Transaction State Machine<br/>INITIATED ➔ POSTED / FAILED]
    end

    subgraph Storage [ACID Relational Storage]
        Service --> DB[(PostgreSQL Store)]
        DB --> RowLock[Row-Level Locking<br/>SELECT FOR UPDATE]
        DB --> OutboxTable[(Transactional Outbox)]
    end

    subgraph Rails [Provider-Agnostic Banking Rails]
        Service --> RailAdapter[Unified Rail Provider]
        RailAdapter --> MockColumn[Mock Column BaaS<br/>ACH / FedNow]
        RailAdapter --> MockWise[Mock Wise Adapter<br/>SWIFT / FX]
    end

    subgraph Events [Asynchronous Event Bus]
        OutboxTable --> OutboxRelay[Outbox Dispatcher]
        OutboxRelay --> EventBus[Kafka / RabbitMQ / CloudEvents]
    end

    Client --> Idemp
```

---

## 🎯 Key Architectural Hallmarks

1. **Spec-Driven Development (SDD):**
   - The OpenAPI 3.1 specification (`api/openapi/v1/ledger.yaml`) is the single source of truth.
   - Router interfaces, request/response DTOs, and field validations are generated via `oapi-codegen` without manual glue code.
2. **Double-Entry Accounting Invariant:**
   - Every movement of money is represented by balanced ledger lines.
   - Currency amounts are stored in **minor integer units** (e.g., cents, pence) to prevent floating-point inaccuracies.
3. **Deterministic Concurrency & Zero Double-Spending:**
   - Account balances are safeguarded by row-level locking (`SELECT ... FOR UPDATE`) and atomic database transactions.
   - Verified by an automated 100-goroutine concurrent stress test.
4. **Strict Idempotency:**
   - Enforces IETF `Idempotency-Key` headers on all mutating endpoints. Concurrent duplicate requests are locked and deduplicated; cached responses are served deterministically.
5. **Provider-Agnostic Banking Rails:**
   - Decoupled from single-provider lock-in. Switching or multi-homing banking partners (e.g. Column, Unit, Stripe Treasury, Wise) requires only a swappable adapter.
6. **Transactional Outbox:**
   - Ensures that domain events are written atomically with ledger state changes, eliminating dual-write distributed failure modes.

---

## 🚀 Getting Started

### Prerequisites
- Go 1.22+
- Make

### Quickstart

```bash
# Clone the repository
git clone https://github.com/akmalkhaniub/apex-ledger.git
cd apex-ledger

# Run tests
make test

# Run race condition and concurrency stress tests
make test-race

# Build and run the server
make run
```

---

## 📜 API Specification

Inspect the contract in `api/openapi/v1/ledger.yaml`.
Includes endpoints for:
- `POST /v1/accounts` (Create accounts with classification: `ASSET`, `LIABILITY`, `EQUITY`, `REVENUE`, `EXPENSE`)
- `GET /v1/accounts/{id}` (Get account metadata)
- `GET /v1/accounts/{id}/balance` (Fetch real-time computed balance)
- `POST /v1/transfers` (Execute multi-line double-entry transfers with `Idempotency-Key`)
- `GET /v1/transfers/{id}` (Fetch transfer state and ledger entries)

---

## 🧪 Verification & Invariant Testing

Run the stress test suite:
```bash
go test -v -race -run TestConcurrentDoubleSpend ./tests/...
```
