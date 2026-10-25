package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const schema = `
CREATE TABLE IF NOT EXISTS wallets (
	id BIGSERIAL PRIMARY KEY,
	owner TEXT NOT NULL,
	balance BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS transfers (
	id BIGSERIAL PRIMARY KEY,
	  idempotency_key TEXT NOT NULL UNIQUE,
  from_wallet     BIGINT NOT NULL REFERENCES wallets(id),
  to_wallet       BIGINT NOT NULL REFERENCES wallets(id),
  amount          BIGINT NOT NULL CHECK (amount > 0),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS transactions (
  id          BIGSERIAL PRIMARY KEY,
  wallet_id   BIGINT NOT NULL REFERENCES wallets(id),
  amount      BIGINT NOT NULL,
  kind        TEXT NOT NULL,
  transfer_id BIGINT REFERENCES transfers(id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

type APP struct {
	db *sql.DB
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	return db, nil
}

func writeJson(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJson(w, status, map[string]string{"error": msg})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

type Wallet struct {
	ID      int64  `json:"id"`
	Owner   string `json:"owner"`
	Balance int64  `json:"balance"`
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (a *APP) createWallet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Owner string `json:"owner"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Owner == "" {
		writeError(w, http.StatusBadRequest, "owner is required")
		return
	}

	var wal Wallet
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO wallets (owner) VALUES ($1) RETURNING id, owner, balance`, req.Owner).Scan(&wal.ID, &wal.Owner, &wal.Balance)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create wallet")
		return
	}
	writeJson(w, http.StatusCreated, wal)
}

func (a *APP) getWallet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}
	var wal Wallet
	err = a.db.QueryRowContext(r.Context(), `SELECT id, owner, balance FROM wallets WHERE id = $1`, id).Scan(&wal.ID, &wal.Owner, &wal.Balance)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "wallet not found")
		return
	}

	writeJson(w, http.StatusOK, wal)
}

func (a *APP) topUp(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid wallet id")
		return
	}

	var req struct {
		Amount int64 `json:"amount"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Amount <= 0 || req.Amount > maxAmount {
		writeError(w, http.StatusBadRequest, "invalid amount")
		return
	}

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback()

	var wal Wallet
	err = tx.QueryRowContext(r.Context(), `UPDATE wallets SET balance = balance + $1 WHERE id = $2 RETURNING id, owner, balance`, req.Amount, id).Scan(&wal.ID, &wal.Owner, &wal.Balance)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "wallet not found")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := tx.ExecContext(r.Context(), `INSERT INTO transactions (wallet_id, amount, kind) VALUES ($1, $2, 'topup')`, id, req.Amount); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJson(w, http.StatusOK, wal)
}

const maxAmount = 10_000_000

func main() {

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:pass@localhost:5433/postgres?sslmode=disable"
	}
	db, err := openDB(dsn)
	if err != nil {
		log.Fatal(err)
	}

	app := &APP{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJson(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /wallets/{id}", app.getWallet)
	mux.HandleFunc("POST /wallets", app.createWallet)
	mux.HandleFunc("POST /wallets/{id}/topup", app.topUp)

	log.Println("listening on port: 8080")
	log.Fatal(http.ListenAndServe(":8080", logging(mux)))
}
