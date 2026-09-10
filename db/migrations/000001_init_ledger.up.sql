-- 000001_init_ledger.up.sql
-- ApexLedger Core Financial Ledger Schema

-- 1. Accounts Table (Chart of Accounts)
CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('ASSET', 'LIABILITY', 'EQUITY', 'REVENUE', 'EXPENSE')),
    normal_balance VARCHAR(10) NOT NULL CHECK (normal_balance IN ('DEBIT', 'CREDIT')),
    currency VARCHAR(3) NOT NULL,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_accounts_currency ON accounts(currency);
CREATE INDEX IF NOT EXISTS idx_accounts_type ON accounts(type);

-- 2. Transfers Table (Transaction Header & State Machine)
CREATE TABLE IF NOT EXISTS transfers (
    id UUID PRIMARY KEY,
    description VARCHAR(255) NOT NULL,
    reference VARCHAR(100),
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'POSTED', 'FAILED', 'REVERSED')),
    rail VARCHAR(20) NOT NULL DEFAULT 'INTERNAL',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    posted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_transfers_status ON transfers(status);
CREATE INDEX IF NOT EXISTS idx_transfers_reference ON transfers(reference);

-- 3. Journal Entries Table (Double-Entry Balanced Lines)
CREATE TABLE IF NOT EXISTS journal_entries (
    id UUID PRIMARY KEY,
    transfer_id UUID NOT NULL REFERENCES transfers(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    direction VARCHAR(10) NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency VARCHAR(3) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_journal_entries_transfer ON journal_entries(transfer_id);
CREATE INDEX IF NOT EXISTS idx_journal_entries_account ON journal_entries(account_id);
CREATE INDEX IF NOT EXISTS idx_journal_entries_created ON journal_entries(created_at);

-- 4. Idempotency Keys (IETF Draft Spec Implementation)
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key UUID PRIMARY KEY,
    request_hash VARCHAR(64) NOT NULL,
    response_status INT,
    response_body TEXT,
    locked_until TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 5. Transactional Outbox (Guaranteed At-Least-Once Async Event Delivery)
CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PUBLISHED', 'FAILED')),
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_pending ON outbox_events(status, created_at) WHERE status = 'PENDING';
