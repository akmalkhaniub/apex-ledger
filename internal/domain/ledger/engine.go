package ledger

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// InvariantValidator guarantees double-entry mathematical consistency
type InvariantValidator struct{}

func NewInvariantValidator() *InvariantValidator {
	return &InvariantValidator{}
}

// Validate verifies that a proposed transfer strictly adheres to double-entry rules
func (v *InvariantValidator) Validate(currency string, entries []JournalEntry) error {
	if len(entries) < 2 {
		return ErrInsufficientEntries
	}

	var totalDebit int64
	var totalCredit int64

	for _, entry := range entries {
		if entry.Amount <= 0 {
			return fmt.Errorf("%w: entry on account %s has amount %d", ErrInvalidAmount, entry.AccountID, entry.Amount)
		}
		if entry.Currency != currency {
			return fmt.Errorf("%w: expected %s, got %s for account %s", ErrCurrencyMismatch, currency, entry.Currency, entry.AccountID)
		}

		switch entry.Direction {
		case DirectionDebit:
			totalDebit += entry.Amount
		case DirectionCredit:
			totalCredit += entry.Amount
		default:
			return fmt.Errorf("invalid direction %s", entry.Direction)
		}
	}

	if totalDebit != totalCredit {
		return fmt.Errorf("%w (total debits: %d, total credits: %d, difference: %d)",
			ErrUnbalancedTransfer, totalDebit, totalCredit, totalDebit-totalCredit)
	}

	return nil
}

// Repository defines the persistence interface for ledgers, accounts, and entries
type Repository interface {
	CreateAccount(ctx context.Context, acc *Account) error
	GetAccount(ctx context.Context, id uuid.UUID) (*Account, error)
	ListAccounts(ctx context.Context, limit int, offset int) ([]Account, int, error)
	GetAccountEntries(ctx context.Context, accountID uuid.UUID) ([]JournalEntry, error)
	SaveTransferAtomic(ctx context.Context, transfer *Transfer, entries []JournalEntry) error
	GetTransfer(ctx context.Context, id uuid.UUID) (*Transfer, error)
	ListTransfers(ctx context.Context, accountID *uuid.UUID, limit int) ([]Transfer, int, error)
}

// Service is the primary business logic orchestrator for accounts and double-entry transfers
type Service struct {
	repo      Repository
	validator *InvariantValidator
	mu        sync.RWMutex
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:      repo,
		validator: NewInvariantValidator(),
	}
}

// CreateAccount registers a new financial account in the chart of accounts
func (s *Service) CreateAccount(ctx context.Context, name string, accType AccountType, currency string, metadata map[string]interface{}) (*Account, error) {
	acc := &Account{
		ID:            uuid.New(),
		Name:          name,
		Type:          accType,
		NormalBalance: DefaultNormalBalance(accType),
		Currency:      currency,
		Metadata:      metadata,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := s.repo.CreateAccount(ctx, acc); err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return acc, nil
}

// GetAccount retrieves account metadata
func (s *Service) GetAccount(ctx context.Context, id uuid.UUID) (*Account, error) {
	return s.repo.GetAccount(ctx, id)
}

// ListAccounts retrieves paginated accounts
func (s *Service) ListAccounts(ctx context.Context, limit int, offset int) ([]Account, int, error) {
	return s.repo.ListAccounts(ctx, limit, offset)
}

// CalculateBalance computes the real-time balance for an account based on all immutable journal entries
func (s *Service) CalculateBalance(ctx context.Context, accountID uuid.UUID) (*AccountBalance, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	entries, err := s.repo.GetAccountEntries(ctx, accountID)
	if err != nil {
		return nil, err
	}

	var totalDebit int64
	var totalCredit int64

	for _, entry := range entries {
		if entry.Direction == DirectionDebit {
			totalDebit += entry.Amount
		} else if entry.Direction == DirectionCredit {
			totalCredit += entry.Amount
		}
	}

	// Double-entry accounting balance calculation based on natural normal balance:
	// - ASSET & EXPENSE: Balance = Debits - Credits
	// - LIABILITY, EQUITY & REVENUE: Balance = Credits - Debits
	var netBalance int64
	if acc.NormalBalance == NormalBalanceDebit {
		netBalance = totalDebit - totalCredit
	} else {
		netBalance = totalCredit - totalDebit
	}

	return &AccountBalance{
		AccountID:        acc.ID,
		Currency:         acc.Currency,
		PostedBalance:    netBalance,
		PendingBalance:   0,
		AvailableBalance: netBalance,
		UpdatedAt:        time.Now().UTC(),
	}, nil
}

// RecordTransfer validates invariants and atomically posts a multi-entry transfer
func (s *Service) RecordTransfer(ctx context.Context, description string, reference string, currency string, rail string, entriesInput []JournalEntry) (*Transfer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Verify double-entry balance: SUM(Debits) == SUM(Credits)
	if err := s.validator.Validate(currency, entriesInput); err != nil {
		return nil, err
	}

	// 2. Validate all accounts exist and currency matches
	for _, entry := range entriesInput {
		acc, err := s.repo.GetAccount(ctx, entry.AccountID)
		if err != nil {
			return nil, fmt.Errorf("account %s verification failed: %w", entry.AccountID, ErrAccountNotFound)
		}
		if acc.Currency != currency {
			return nil, fmt.Errorf("%w: account %s is %s, transfer is %s", ErrCurrencyMismatch, entry.AccountID, acc.Currency, currency)
		}

		// Prevent overdrafts on asset accounts if normal balance is debit and drawing below zero
		if acc.Type == AccountTypeAsset && entry.Direction == DirectionCredit {
			bal, err := s.CalculateBalance(ctx, acc.ID)
			if err != nil {
				return nil, err
			}
			if bal.AvailableBalance < entry.Amount {
				return nil, fmt.Errorf("%w: account %s has available %d, required %d",
					ErrInsufficientFunds, acc.ID, bal.AvailableBalance, entry.Amount)
			}
		}
	}

	now := time.Now().UTC()
	transferID := uuid.New()

	transfer := &Transfer{
		ID:          transferID,
		Description: description,
		Reference:   reference,
		Currency:    currency,
		Status:      TransferStatusPosted,
		Rail:        rail,
		CreatedAt:   now,
		PostedAt:    &now,
	}

	entries := make([]JournalEntry, len(entriesInput))
	for i, e := range entriesInput {
		entries[i] = JournalEntry{
			ID:         uuid.New(),
			TransferID: transferID,
			AccountID:  e.AccountID,
			Direction:  e.Direction,
			Amount:     e.Amount,
			Currency:   currency,
			CreatedAt:  now,
		}
	}
	transfer.Entries = entries

	// 3. Atomically persist transfer and all journal entries
	if err := s.repo.SaveTransferAtomic(ctx, transfer, entries); err != nil {
		return nil, fmt.Errorf("atomic persistence failed: %w", err)
	}

	return transfer, nil
}

func (s *Service) GetTransfer(ctx context.Context, id uuid.UUID) (*Transfer, error) {
	return s.repo.GetTransfer(ctx, id)
}

func (s *Service) ListTransfers(ctx context.Context, accountID *uuid.UUID, limit int) ([]Transfer, int, error) {
	return s.repo.ListTransfers(ctx, accountID, limit)
}
