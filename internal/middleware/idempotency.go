package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/akmalkhaniub/apex-ledger/internal/generated/api"
	"github.com/akmalkhaniub/apex-ledger/internal/repository"
	"github.com/google/uuid"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (rec *responseRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *responseRecorder) Write(b []byte) (int, error) {
	rec.body.Write(b)
	return rec.ResponseWriter.Write(b)
}

// IdempotencyMiddleware guarantees exactly-once request processing conforming to IETF specs
func IdempotencyMiddleware(repo *repository.MemoryRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only mutating methods require idempotency enforcement
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			keyHeader := r.Header.Get("Idempotency-Key")
			if keyHeader == "" {
				api.RespondProblem(w, http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required for mutating requests")
				return
			}

			key, err := uuid.Parse(keyHeader)
			if err != nil {
				api.RespondProblem(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key header must be a valid UUIDv4")
				return
			}

			// Read body to compute request fingerprint hash
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				api.RespondProblem(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "Could not read request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			hash := sha256.Sum256(bodyBytes)
			hashStr := hex.EncodeToString(hash[:])

			// Try to acquire atomic lock
			acquired, record := repo.TryLockIdempotency(key, hashStr, 30*time.Second)
			if !acquired {
				if record != nil && record.ResponseStatus != 0 {
					// Verify request hash matches original
					if record.RequestHash != hashStr {
						api.RespondProblem(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_PAYLOAD_MISMATCH", "Payload does not match original request for this Idempotency-Key")
						return
					}

					// Replay cached response
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Cache-Lookup", "HIT")
					w.WriteHeader(record.ResponseStatus)
					_, _ = w.Write([]byte(record.ResponseBody))
					return
				}

				// Concurrent request currently in flight
				api.RespondProblem(w, http.StatusConflict, "CONCURRENT_REQUEST_IN_FLIGHT", "A request with this Idempotency-Key is currently processing")
				return
			}

			// Intercept and record response
			rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Resolve lock and cache response
			repo.ResolveIdempotency(key, rec.statusCode, rec.body.String())
		})
	}
}
