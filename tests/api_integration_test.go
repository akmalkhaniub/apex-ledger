package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/akmalkhaniub/apex-ledger/internal/api"
	"github.com/akmalkhaniub/apex-ledger/internal/domain/ledger"
	gen "github.com/akmalkhaniub/apex-ledger/internal/generated/api"
	"github.com/akmalkhaniub/apex-ledger/internal/middleware"
	"github.com/akmalkhaniub/apex-ledger/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupTestServer() (http.Handler, *repository.MemoryRepository) {
	repo := repository.NewMemoryRepository()
	service := ledger.NewService(repo)
	apiHandler := api.NewLedgerAPI(service)

	r := chi.NewRouter()
	r.Use(middleware.IdempotencyMiddleware(repo))
	gen.HandlerFromMux(apiHandler, r)

	return r, repo
}

func TestE2E_APIFlowAndIdempotency(t *testing.T) {
	handler, _ := setupTestServer()

	// 1. Create Customer Account via POST /v1/accounts
	accReqBody := []byte(`{
		"name": "Alex Tech LLC Operating",
		"type": "ASSET",
		"currency": "USD"
	}`)

	idempKey := uuid.New().String()

	req, _ := http.NewRequest(http.MethodPost, "/v1/accounts", bytes.NewBuffer(accReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempKey)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var customerAccount gen.Account
	if err := json.Unmarshal(rec.Body.Bytes(), &customerAccount); err != nil {
		t.Fatalf("failed to decode account response: %v", err)
	}

	if customerAccount.Name != "Alex Tech LLC Operating" {
		t.Errorf("expected account name 'Alex Tech LLC Operating', got %s", customerAccount.Name)
	}

	// 2. Test Idempotency Replay: Send identical request with same key
	req2, _ := http.NewRequest(http.MethodPost, "/v1/accounts", bytes.NewBuffer(accReqBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", idempKey)

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected 201 for idempotency replay, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-Cache-Lookup") != "HIT" {
		t.Errorf("expected X-Cache-Lookup HIT header on replayed request")
	}

	var replayedAccount gen.Account
	_ = json.Unmarshal(rec2.Body.Bytes(), &replayedAccount)
	if replayedAccount.Id != customerAccount.Id {
		t.Errorf("expected identical account ID %s, got %s", customerAccount.Id, replayedAccount.Id)
	}

	// 3. Create Equity Reserve Account
	reserveReqBody := []byte(`{
		"name": "Founding Bank Capital",
		"type": "EQUITY",
		"currency": "USD"
	}`)
	reqReserve, _ := http.NewRequest(http.MethodPost, "/v1/accounts", bytes.NewBuffer(reserveReqBody))
	reqReserve.Header.Set("Content-Type", "application/json")
	reqReserve.Header.Set("Idempotency-Key", uuid.New().String())

	recReserve := httptest.NewRecorder()
	handler.ServeHTTP(recReserve, reqReserve)

	var reserveAccount gen.Account
	_ = json.Unmarshal(recReserve.Body.Bytes(), &reserveAccount)

	// 4. Post Balanced Transfer: $250.00 (25,000 cents)
	transferReqBody := map[string]interface{}{
		"description": "Initial Seed Deposit",
		"currency":    "USD",
		"rail":        "ACH",
		"entries": []map[string]interface{}{
			{
				"account_id": customerAccount.Id,
				"direction":  "DEBIT",
				"amount":     25000,
			},
			{
				"account_id": reserveAccount.Id,
				"direction":  "CREDIT",
				"amount":     25000,
			},
		},
	}
	trBytes, _ := json.Marshal(transferReqBody)

	trReq, _ := http.NewRequest(http.MethodPost, "/v1/transfers", bytes.NewBuffer(trBytes))
	trReq.Header.Set("Content-Type", "application/json")
	trReq.Header.Set("Idempotency-Key", uuid.New().String())

	trRec := httptest.NewRecorder()
	handler.ServeHTTP(trRec, trReq)

	if trRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for balanced transfer, got %d: %s", trRec.Code, trRec.Body.String())
	}

	// 5. Query Balance for Customer Account: GET /v1/accounts/{id}/balance
	balReq, _ := http.NewRequest(http.MethodGet, "/v1/accounts/"+customerAccount.Id.String()+"/balance", nil)
	balRec := httptest.NewRecorder()
	handler.ServeHTTP(balRec, balReq)

	if balRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for balance check, got %d", balRec.Code)
	}

	var balance gen.AccountBalance
	_ = json.Unmarshal(balRec.Body.Bytes(), &balance)
	if balance.PostedBalance != 25000 {
		t.Errorf("expected posted balance 25000, got %d", balance.PostedBalance)
	}

	// 6. Test Unbalanced Transfer Rejection: Debits != Credits
	fraudTransferBody := map[string]interface{}{
		"description": "Unbalanced Transaction",
		"currency":    "USD",
		"entries": []map[string]interface{}{
			{"account_id": customerAccount.Id, "direction": "DEBIT", "amount": 25000},
			{"account_id": reserveAccount.Id, "direction": "CREDIT", "amount": 10000},
		},
	}
	fraudBytes, _ := json.Marshal(fraudTransferBody)
	fraudReq, _ := http.NewRequest(http.MethodPost, "/v1/transfers", bytes.NewBuffer(fraudBytes))
	fraudReq.Header.Set("Content-Type", "application/json")
	fraudReq.Header.Set("Idempotency-Key", uuid.New().String())

	fraudRec := httptest.NewRecorder()
	handler.ServeHTTP(fraudRec, fraudReq)

	if fraudRec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 Unprocessable Entity, got %d", fraudRec.Code)
	}

	var problem gen.ProblemDetails
	_ = json.Unmarshal(fraudRec.Body.Bytes(), &problem)
	if problem.Code != "LEDGER_INVARIANT_VIOLATION" {
		t.Errorf("expected error code LEDGER_INVARIANT_VIOLATION, got %s", problem.Code)
	}
}
