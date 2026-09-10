package tests

import (
	"context"
	"testing"

	"github.com/akmalkhaniub/apex-ledger/internal/domain/ledger"
	"github.com/akmalkhaniub/apex-ledger/internal/repository"
)

func setupTestService() (*ledger.Service, *repository.MemoryRepository) {
	repo := repository.NewMemoryRepository()
	service := ledger.NewService(repo)
	return service, repo
}

func TestDoubleEntryInvariant_Success(t *testing.T) {
	service, _ := setupTestService()
	ctx := context.Background()

	// 1. Create accounts: Asset (Customer Wallet) & Equity (Bank Reserve)
	custWallet, err := service.CreateAccount(ctx, "Customer Checking", ledger.AccountTypeAsset, "USD", nil)
	if err != nil {
		t.Fatalf("failed to create customer wallet: %v", err)
	}

	bankReserve, err := service.CreateAccount(ctx, "Initial Bank Equity", ledger.AccountTypeEquity, "USD", nil)
	if err != nil {
		t.Fatalf("failed to create bank reserve: %v", err)
	}

	// 2. Deposit $500 (50,000 cents) into Customer Wallet:
	// Debit Asset (+50,000), Credit Equity (+50,000)
	entries := []ledger.JournalEntry{
		{AccountID: custWallet.ID, Direction: ledger.DirectionDebit, Amount: 50000, Currency: "USD"},
		{AccountID: bankReserve.ID, Direction: ledger.DirectionCredit, Amount: 50000, Currency: "USD"},
	}

	transfer, err := service.RecordTransfer(ctx, "Initial Customer Funding", "ref_001", "USD", "INTERNAL", entries)
	if err != nil {
		t.Fatalf("balanced transfer should succeed, got: %v", err)
	}

	if transfer.Status != ledger.TransferStatusPosted {
		t.Errorf("expected transfer status POSTED, got %s", transfer.Status)
	}

	// Verify Balances
	custBal, err := service.CalculateBalance(ctx, custWallet.ID)
	if err != nil {
		t.Fatalf("failed to compute customer balance: %v", err)
	}
	if custBal.PostedBalance != 50000 {
		t.Errorf("expected customer balance 50000, got %d", custBal.PostedBalance)
	}

	equityBal, err := service.CalculateBalance(ctx, bankReserve.ID)
	if err != nil {
		t.Fatalf("failed to compute equity balance: %v", err)
	}
	if equityBal.PostedBalance != 50000 {
		t.Errorf("expected equity balance 50000, got %d", equityBal.PostedBalance)
	}
}

func TestDoubleEntryInvariant_UnbalancedRejection(t *testing.T) {
	service, _ := setupTestService()
	ctx := context.Background()

	custWallet, _ := service.CreateAccount(ctx, "Customer A", ledger.AccountTypeAsset, "USD", nil)
	bankReserve, _ := service.CreateAccount(ctx, "Bank Reserve", ledger.AccountTypeEquity, "USD", nil)

	// Unbalanced: Debit 50,000 vs Credit 40,000 (diff: 10,000)
	entries := []ledger.JournalEntry{
		{AccountID: custWallet.ID, Direction: ledger.DirectionDebit, Amount: 50000, Currency: "USD"},
		{AccountID: bankReserve.ID, Direction: ledger.DirectionCredit, Amount: 40000, Currency: "USD"},
	}

	_, err := service.RecordTransfer(ctx, "Unbalanced Fraud Attempt", "ref_002", "USD", "INTERNAL", entries)
	if err == nil {
		t.Fatal("expected unbalanced transfer to be rejected, but it succeeded")
	}
}

func TestDoubleEntryInvariant_CurrencyMismatch(t *testing.T) {
	service, _ := setupTestService()
	ctx := context.Background()

	custUSD, _ := service.CreateAccount(ctx, "USD Wallet", ledger.AccountTypeAsset, "USD", nil)
	custEUR, _ := service.CreateAccount(ctx, "EUR Wallet", ledger.AccountTypeAsset, "EUR", nil)

	entries := []ledger.JournalEntry{
		{AccountID: custUSD.ID, Direction: ledger.DirectionDebit, Amount: 1000, Currency: "USD"},
		{AccountID: custEUR.ID, Direction: ledger.DirectionCredit, Amount: 1000, Currency: "USD"},
	}

	_, err := service.RecordTransfer(ctx, "Currency Mismatch Attempt", "ref_003", "USD", "INTERNAL", entries)
	if err == nil {
		t.Fatal("expected cross-currency account error, but it succeeded")
	}
}

func TestDoubleEntryInvariant_InsufficientFunds(t *testing.T) {
	service, _ := setupTestService()
	ctx := context.Background()

	sender, _ := service.CreateAccount(ctx, "Sender Wallet", ledger.AccountTypeAsset, "USD", nil)
	receiver, _ := service.CreateAccount(ctx, "Receiver Wallet", ledger.AccountTypeAsset, "USD", nil)

	// Sender starts with $0 balance, attempting to credit/spend $100
	entries := []ledger.JournalEntry{
		{AccountID: sender.ID, Direction: ledger.DirectionCredit, Amount: 10000, Currency: "USD"},
		{AccountID: receiver.ID, Direction: ledger.DirectionDebit, Amount: 10000, Currency: "USD"},
	}

	_, err := service.RecordTransfer(ctx, "Overdraft Attempt", "ref_004", "USD", "INTERNAL", entries)
	if err == nil {
		t.Fatal("expected overdraft transfer to be rejected, but it succeeded")
	}
}
