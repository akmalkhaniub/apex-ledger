-- name: CreateAccount :one
INSERT INTO accounts (id, name, type, normal_balance, currency, metadata, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts
WHERE id = $1 LIMIT 1;

-- name: GetAccountForUpdate :one
SELECT * FROM accounts
WHERE id = $1 LIMIT 1
FOR UPDATE;

-- name: ListAccounts :many
SELECT * FROM accounts
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CreateTransfer :one
INSERT INTO transfers (id, description, reference, currency, status, rail, created_at, posted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetTransfer :one
SELECT * FROM transfers
WHERE id = $1 LIMIT 1;

-- name: UpdateTransferStatus :one
UPDATE transfers
SET status = $2, posted_at = $3
WHERE id = $1
RETURNING *;

-- name: CreateJournalEntry :one
INSERT INTO journal_entries (id, transfer_id, account_id, direction, amount, currency, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetJournalEntriesByTransfer :many
SELECT * FROM journal_entries
WHERE transfer_id = $1
ORDER BY created_at ASC;

-- name: GetAccountJournalEntries :many
SELECT * FROM journal_entries
WHERE account_id = $1
ORDER BY created_at ASC;

-- name: GetIdempotencyRecord :one
SELECT * FROM idempotency_keys
WHERE key = $1 LIMIT 1;

-- name: CreateIdempotencyLock :one
INSERT INTO idempotency_keys (key, request_hash, locked_until, created_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ResolveIdempotencyLock :exec
UPDATE idempotency_keys
SET response_status = $2, response_body = $3
WHERE key = $1;

-- name: CreateOutboxEvent :one
INSERT INTO outbox_events (id, event_type, payload, status, retry_count, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: FetchPendingOutboxEvents :many
SELECT * FROM outbox_events
WHERE status = 'PENDING'
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxEventPublished :exec
UPDATE outbox_events
SET status = 'PUBLISHED', published_at = $2
WHERE id = $1;
