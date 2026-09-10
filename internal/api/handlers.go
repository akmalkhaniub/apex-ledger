package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/akmalkhaniub/apex-ledger/internal/domain/ledger"
	gen "github.com/akmalkhaniub/apex-ledger/internal/generated/api"
	"github.com/google/uuid"
)

// LedgerAPI implements gen.ServerInterface
type LedgerAPI struct {
	service *ledger.Service
}

func NewLedgerAPI(service *ledger.Service) *LedgerAPI {
	return &LedgerAPI{service: service}
}

// CreateAccount handles POST /v1/accounts
func (a *LedgerAPI) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req gen.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		gen.RespondProblem(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid JSON request payload")
		return
	}

	if req.Name == "" || req.Currency == "" || req.Type == "" {
		gen.RespondProblem(w, http.StatusBadRequest, "VALIDATION_FAILED", "name, currency, and type are required")
		return
	}

	acc, err := a.service.CreateAccount(r.Context(), req.Name, ledger.AccountType(req.Type), req.Currency, req.Metadata)
	if err != nil {
		gen.RespondProblem(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	res := gen.Account{
		Id:            acc.ID,
		Name:          acc.Name,
		Type:          gen.AccountType(acc.Type),
		NormalBalance: gen.NormalBalance(acc.NormalBalance),
		Currency:      acc.Currency,
		Metadata:      acc.Metadata,
		CreatedAt:     acc.CreatedAt,
		UpdatedAt:     &acc.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}

// ListAccounts handles GET /v1/accounts
func (a *LedgerAPI) ListAccounts(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	accounts, total, err := a.service.ListAccounts(r.Context(), limit, 0)
	if err != nil {
		gen.RespondProblem(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]gen.Account, len(accounts))
	for i, acc := range accounts {
		items[i] = gen.Account{
			Id:            acc.ID,
			Name:          acc.Name,
			Type:          gen.AccountType(acc.Type),
			NormalBalance: gen.NormalBalance(acc.NormalBalance),
			Currency:      acc.Currency,
			CreatedAt:     acc.CreatedAt,
		}
	}

	res := gen.AccountList{
		Items: items,
		Total: total,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// GetAccount handles GET /v1/accounts/{accountId}
func (a *LedgerAPI) GetAccount(w http.ResponseWriter, r *http.Request, accountId uuid.UUID) {
	acc, err := a.service.GetAccount(r.Context(), accountId)
	if err != nil {
		gen.RespondProblem(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "Account not found")
		return
	}

	res := gen.Account{
		Id:            acc.ID,
		Name:          acc.Name,
		Type:          gen.AccountType(acc.Type),
		NormalBalance: gen.NormalBalance(acc.NormalBalance),
		Currency:      acc.Currency,
		CreatedAt:     acc.CreatedAt,
		UpdatedAt:     &acc.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// GetAccountBalance handles GET /v1/accounts/{accountId}/balance
func (a *LedgerAPI) GetAccountBalance(w http.ResponseWriter, r *http.Request, accountId uuid.UUID) {
	bal, err := a.service.CalculateBalance(r.Context(), accountId)
	if err != nil {
		gen.RespondProblem(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", err.Error())
		return
	}

	res := gen.AccountBalance{
		AccountId:        bal.AccountID,
		Currency:         bal.Currency,
		PostedBalance:    bal.PostedBalance,
		PendingBalance:   bal.PendingBalance,
		AvailableBalance: bal.AvailableBalance,
		UpdatedAt:        bal.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// CreateTransfer handles POST /v1/transfers
func (a *LedgerAPI) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	var req gen.CreateTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		gen.RespondProblem(w, http.StatusBadRequest, "MALFORMED_JSON", "Invalid JSON transfer payload")
		return
	}

	if len(req.Entries) < 2 {
		gen.RespondProblem(w, http.StatusBadRequest, "INSUFFICIENT_ENTRIES", "A transfer must contain at least 2 entries")
		return
	}

	entries := make([]ledger.JournalEntry, len(req.Entries))
	for i, e := range req.Entries {
		entries[i] = ledger.JournalEntry{
			AccountID: e.AccountId,
			Direction: ledger.EntryDirection(e.Direction),
			Amount:    e.Amount,
			Currency:  req.Currency,
		}
	}

	ref := ""
	if req.Reference != nil {
		ref = *req.Reference
	}

	rail := "INTERNAL"
	if req.Rail != nil {
		rail = *req.Rail
	}

	transfer, err := a.service.RecordTransfer(r.Context(), req.Description, ref, req.Currency, rail, entries)
	if err != nil {
		gen.RespondProblem(w, http.StatusUnprocessableEntity, "LEDGER_INVARIANT_VIOLATION", err.Error())
		return
	}

	outEntries := make([]gen.JournalEntry, len(transfer.Entries))
	for i, e := range transfer.Entries {
		outEntries[i] = gen.JournalEntry{
			Id:         e.ID,
			TransferId: e.TransferID,
			AccountId:  e.AccountID,
			Direction:  gen.EntryDirection(e.Direction),
			Amount:     e.Amount,
			Currency:   e.Currency,
			CreatedAt:  e.CreatedAt,
		}
	}

	res := gen.Transfer{
		Id:          transfer.ID,
		Description: transfer.Description,
		Reference:   &transfer.Reference,
		Currency:    transfer.Currency,
		Status:      gen.TransferStatus(transfer.Status),
		Rail:        transfer.Rail,
		Entries:     outEntries,
		CreatedAt:   transfer.CreatedAt,
		PostedAt:    transfer.PostedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}

// ListTransfers handles GET /v1/transfers
func (a *LedgerAPI) ListTransfers(w http.ResponseWriter, r *http.Request) {
	transfers, total, err := a.service.ListTransfers(r.Context(), nil, 50)
	if err != nil {
		gen.RespondProblem(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]gen.Transfer, len(transfers))
	for i, t := range transfers {
		items[i] = gen.Transfer{
			Id:          t.ID,
			Description: t.Description,
			Currency:    t.Currency,
			Status:      gen.TransferStatus(t.Status),
			Rail:        t.Rail,
			CreatedAt:   t.CreatedAt,
		}
	}

	res := gen.TransferList{
		Items: items,
		Total: total,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// GetTransfer handles GET /v1/transfers/{transferId}
func (a *LedgerAPI) GetTransfer(w http.ResponseWriter, r *http.Request, transferId uuid.UUID) {
	transfer, err := a.service.GetTransfer(r.Context(), transferId)
	if err != nil {
		gen.RespondProblem(w, http.StatusNotFound, "TRANSFER_NOT_FOUND", "Transfer not found")
		return
	}

	res := gen.Transfer{
		Id:          transfer.ID,
		Description: transfer.Description,
		Reference:   &transfer.Reference,
		Currency:    transfer.Currency,
		Status:      gen.TransferStatus(transfer.Status),
		Rail:        transfer.Rail,
		CreatedAt:   transfer.CreatedAt,
		PostedAt:    transfer.PostedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
