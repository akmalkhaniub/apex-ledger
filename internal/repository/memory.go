package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/akmalkhaniub/apex-ledger/internal/domain/ledger"
	"github.com/google/uuid"
)

// MemoryRepository provides a concurrent, in-memory double-entry store
type MemoryRepository struct {
	mu             sync.RWMutex
	accounts       map[uuid.UUID]*ledger.Account
	transfers      map[uuid.UUID]*ledger.Transfer
	journalEntries []ledger.JournalEntry
	accountEntries map[uuid.UUID][]ledger.JournalEntry
	idempotency    map[uuid.UUID]*IdempotencyRecord
	outbox         []OutboxRecord
}

type IdempotencyRecord struct {
	Key            uuid.UUID
	RequestHash    string
	ResponseStatus int
	ResponseBody   string
	LockedUntil    time.Time
	CreatedAt      time.Time
}

type OutboxRecord struct {
	ID        uuid.UUID
	EventType string
	Payload   string
	Status    string
	CreatedAt time.Time
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		accounts:       make(map[uuid.UUID]*ledger.Account),
		transfers:      make(map[uuid.UUID]*ledger.Transfer),
		accountEntries: make(map[uuid.UUID][]ledger.JournalEntry),
		idempotency:    make(map[uuid.UUID]*IdempotencyRecord),
	}
}

func (m *MemoryRepository) CreateAccount(ctx context.Context, acc *ledger.Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.accounts[acc.ID]; exists {
		return fmt.Errorf("account %s already exists", acc.ID)
	}

	m.accounts[acc.ID] = acc
	m.accountEntries[acc.ID] = make([]ledger.JournalEntry, 0)
	return nil
}

func (m *MemoryRepository) GetAccount(ctx context.Context, id uuid.UUID) (*ledger.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	acc, exists := m.accounts[id]
	if !exists {
		return nil, ledger.ErrAccountNotFound
	}

	// return copy
	c := *acc
	return &c, nil
}

func (m *MemoryRepository) ListAccounts(ctx context.Context, limit int, offset int) ([]ledger.Account, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := len(m.accounts)
	items := make([]ledger.Account, 0, limit)

	idx := 0
	for _, acc := range m.accounts {
		if idx >= offset && len(items) < limit {
			items = append(items, *acc)
		}
		idx++
	}

	return items, total, nil
}

func (m *MemoryRepository) GetAccountEntries(ctx context.Context, accountID uuid.UUID) ([]ledger.JournalEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, exists := m.accountEntries[accountID]
	if !exists {
		return nil, ledger.ErrAccountNotFound
	}

	res := make([]ledger.JournalEntry, len(entries))
	copy(res, entries)
	return res, nil
}

func (m *MemoryRepository) SaveTransferAtomic(ctx context.Context, transfer *ledger.Transfer, entries []ledger.JournalEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify all accounts exist
	for _, entry := range entries {
		if _, exists := m.accounts[entry.AccountID]; !exists {
			return fmt.Errorf("%w: %s", ledger.ErrAccountNotFound, entry.AccountID)
		}
	}

	// Persist transfer header
	m.transfers[transfer.ID] = transfer

	// Append journal entries atomically
	for _, entry := range entries {
		m.journalEntries = append(m.journalEntries, entry)
		m.accountEntries[entry.AccountID] = append(m.accountEntries[entry.AccountID], entry)
	}

	// Add to outbox atomically
	m.outbox = append(m.outbox, OutboxRecord{
		ID:        uuid.New(),
		EventType: "ledger.transfer.posted",
		Payload:   fmt.Sprintf(`{"transfer_id":"%s","currency":"%s"}`, transfer.ID, transfer.Currency),
		Status:    "PENDING",
		CreatedAt: time.Now().UTC(),
	})

	return nil
}

func (m *MemoryRepository) GetTransfer(ctx context.Context, id uuid.UUID) (*ledger.Transfer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tr, exists := m.transfers[id]
	if !exists {
		return nil, fmt.Errorf("transfer %s not found", id)
	}

	c := *tr
	return &c, nil
}

func (m *MemoryRepository) ListTransfers(ctx context.Context, accountID *uuid.UUID, limit int) ([]ledger.Transfer, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := len(m.transfers)
	items := make([]ledger.Transfer, 0, limit)

	for _, tr := range m.transfers {
		if accountID != nil {
			match := false
			for _, e := range tr.Entries {
				if e.AccountID == *accountID {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}

		if len(items) < limit {
			items = append(items, *tr)
		}
	}

	return items, total, nil
}

// Idempotency methods
func (m *MemoryRepository) TryLockIdempotency(key uuid.UUID, hash string, lockTTL time.Duration) (bool, *IdempotencyRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec, exists := m.idempotency[key]
	now := time.Now().UTC()

	if exists {
		// If already resolved (response cached)
		if rec.ResponseStatus != 0 {
			return false, rec
		}
		// If currently locked and lock hasn't expired
		if rec.LockedUntil.After(now) {
			return false, rec
		}
	}

	// Acquire lock
	record := &IdempotencyRecord{
		Key:         key,
		RequestHash: hash,
		LockedUntil: now.Add(lockTTL),
		CreatedAt:   now,
	}
	m.idempotency[key] = record
	return true, nil
}

func (m *MemoryRepository) ResolveIdempotency(key uuid.UUID, status int, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rec, exists := m.idempotency[key]; exists {
		rec.ResponseStatus = status
		rec.ResponseBody = body
	}
}
