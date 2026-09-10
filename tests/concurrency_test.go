package tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/akmalkhaniub/apex-ledger/internal/domain/ledger"
)

// TestConcurrentDoubleSpend spins up 100 concurrent goroutines attempting to withdraw
// from an account that only has funds for exactly 2 withdrawals.
// Verifies that exactly 2 succeed, 98 fail, and final balance is strictly $0.
func TestConcurrentDoubleSpend(t *testing.T) {
	service, _ := setupTestService()
	ctx := context.Background()

	// 1. Setup accounts
	sender, err := service.CreateAccount(ctx, "High-Velocity Customer", ledger.AccountTypeAsset, "USD", nil)
	if err != nil {
		t.Fatalf("failed to create sender account: %v", err)
	}

	merchant, err := service.CreateAccount(ctx, "Merchant Terminal", ledger.AccountTypeAsset, "USD", nil)
	if err != nil {
		t.Fatalf("failed to create merchant account: %v", err)
	}

	equity, err := service.CreateAccount(ctx, "Funding Source", ledger.AccountTypeEquity, "USD", nil)
	if err != nil {
		t.Fatalf("failed to create equity account: %v", err)
	}

	// 2. Fund sender with exactly $100 (10,000 cents)
	fundingEntries := []ledger.JournalEntry{
		{AccountID: sender.ID, Direction: ledger.DirectionDebit, Amount: 10000, Currency: "USD"},
		{AccountID: equity.ID, Direction: ledger.DirectionCredit, Amount: 10000, Currency: "USD"},
	}
	_, err = service.RecordTransfer(ctx, "Funding 100 USD", "fund_001", "USD", "INTERNAL", fundingEntries)
	if err != nil {
		t.Fatalf("initial funding failed: %v", err)
	}

	// 3. Launch 100 concurrent goroutines, each trying to transfer $50 (5,000 cents)
	const concurrentClients = 100
	const withdrawalAmount = 5000 // $50

	var wg sync.WaitGroup
	var successfulTransfers int64
	var failedTransfers int64

	// Barrier to ensure all goroutines start simultaneously
	startBarrier := make(chan struct{})

	for i := 0; i < concurrentClients; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			<-startBarrier // wait for synchronized blast

			entries := []ledger.JournalEntry{
				{AccountID: sender.ID, Direction: ledger.DirectionCredit, Amount: withdrawalAmount, Currency: "USD"},
				{AccountID: merchant.ID, Direction: ledger.DirectionDebit, Amount: withdrawalAmount, Currency: "USD"},
			}

			ref := fmt.Sprintf("race_tx_%d", workerID)
			_, err := service.RecordTransfer(ctx, "Concurrent Debit", ref, "USD", "INTERNAL", entries)
			if err == nil {
				atomic.AddInt64(&successfulTransfers, 1)
			} else {
				atomic.AddInt64(&failedTransfers, 1)
			}
		}(i)
	}

	// Fire all goroutines simultaneously
	close(startBarrier)
	wg.Wait()

	// 4. Assert Invariants
	t.Logf("Stress test complete. Successes: %d, Failures: %d", successfulTransfers, failedTransfers)

	if successfulTransfers != 2 {
		t.Errorf("CRITICAL INVARIANT VIOLATION: Expected exactly 2 successful withdrawals, got %d", successfulTransfers)
	}

	if failedTransfers != concurrentClients-2 {
		t.Errorf("Expected %d failed withdrawals, got %d", concurrentClients-2, failedTransfers)
	}

	// 5. Verify final balance is exactly 0 cents
	finalSenderBal, err := service.CalculateBalance(ctx, sender.ID)
	if err != nil {
		t.Fatalf("failed to calculate sender balance: %v", err)
	}
	if finalSenderBal.PostedBalance != 0 {
		t.Errorf("CRITICAL: Sender balance expected 0, but is %d (Double-spend occurred!)", finalSenderBal.PostedBalance)
	}

	finalMerchantBal, err := service.CalculateBalance(ctx, merchant.ID)
	if err != nil {
		t.Fatalf("failed to calculate merchant balance: %v", err)
	}
	if finalMerchantBal.PostedBalance != 10000 {
		t.Errorf("Merchant balance expected 10000 cents ($100), got %d", finalMerchantBal.PostedBalance)
	}
}
