package ledger

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Standard Chart of Accounts categorization
type AccountType string

const (
	AccountTypeAsset     AccountType = "ASSET"
	AccountTypeLiability AccountType = "LIABILITY"
	AccountTypeEquity    AccountType = "EQUITY"
	AccountTypeRevenue   AccountType = "REVENUE"
	AccountTypeExpense   AccountType = "EXPENSE"
)

// Normal balance rule for double-entry bookkeeping
type NormalBalance string

const (
	NormalBalanceDebit  NormalBalance = "DEBIT"
	NormalBalanceCredit NormalBalance = "CREDIT"
)

// Entry direction (Debit or Credit)
type EntryDirection string

const (
	DirectionDebit  EntryDirection = "DEBIT"
	DirectionCredit EntryDirection = "CREDIT"
)

// Transfer state machine
type TransferStatus string

const (
	TransferStatusPending  TransferStatus = "PENDING"
	TransferStatusPosted   TransferStatus = "POSTED"
	TransferStatusFailed   TransferStatus = "FAILED"
	TransferStatusReversed TransferStatus = "REVERSED"
)

// Account represents an entity in the Chart of Accounts
type Account struct {
	ID            uuid.UUID              `json:"id"`
	Name          string                 `json:"name"`
	Type          AccountType            `json:"type"`
	NormalBalance NormalBalance          `json:"normal_balance"`
	Currency      string                 `json:"currency"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// DefaultNormalBalance returns the natural normal balance for a given account type
func DefaultNormalBalance(t AccountType) NormalBalance {
	switch t {
	case AccountTypeAsset, AccountTypeExpense:
		return NormalBalanceDebit
	case AccountTypeLiability, AccountTypeEquity, AccountTypeRevenue:
		return NormalBalanceCredit
	default:
		return NormalBalanceDebit
	}
}

// JournalEntry represents one line in a balanced double-entry transaction
type JournalEntry struct {
	ID         uuid.UUID      `json:"id"`
	TransferID uuid.UUID      `json:"transfer_id"`
	AccountID  uuid.UUID      `json:"account_id"`
	Direction  EntryDirection `json:"direction"`
	Amount     int64          `json:"amount"` // Positive integer in minor units (cents)
	Currency   string         `json:"currency"`
	CreatedAt  time.Time      `json:"created_at"`
}

// Transfer represents the atomic double-entry transaction header
type Transfer struct {
	ID          uuid.UUID      `json:"id"`
	Description string         `json:"description"`
	Reference   string         `json:"reference,omitempty"`
	Currency    string         `json:"currency"`
	Status      TransferStatus `json:"status"`
	Rail        string         `json:"rail"`
	Entries     []JournalEntry `json:"entries"`
	CreatedAt   time.Time      `json:"created_at"`
	PostedAt    *time.Time     `json:"posted_at,omitempty"`
}

// AccountBalance represents the computed balance of an account
type AccountBalance struct {
	AccountID        uuid.UUID `json:"account_id"`
	Currency         string    `json:"currency"`
	PostedBalance    int64     `json:"posted_balance"`
	PendingBalance   int64     `json:"pending_balance"`
	AvailableBalance int64     `json:"available_balance"`
	UpdatedAt        time.Time `json:"updated_at"`
}

var (
	ErrUnbalancedTransfer     = errors.New("unbalanced transfer: sum of debits must equal sum of credits")
	ErrInvalidAmount          = errors.New("entry amount must be strictly positive")
	ErrInsufficientEntries    = errors.New("a double-entry transfer must have at least 2 entries")
	ErrCurrencyMismatch       = errors.New("all entries must match the transfer currency")
	ErrInsufficientFunds      = errors.New("insufficient funds for asset account debit/withdrawal")
	ErrAccountNotFound        = errors.New("account not found")
	ErrInvalidStateTransition = errors.New("invalid transfer state transition")
)
