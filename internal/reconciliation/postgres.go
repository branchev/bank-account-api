package reconciliation

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresService struct {
	db *pgxpool.Pool
}

func NewPostgresService(db *pgxpool.Pool) *PostgresService {
	return &PostgresService{db: db}
}

func (s *PostgresService) Reconcile(ctx context.Context) (Report, error) {
	report := Report{
		Status:        "consistent",
		Discrepancies: []Discrepancy{},
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return Report{}, fmt.Errorf("begin reconciliation: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT
			a.id,
			a.balance_cents,
			COALESCE(SUM(le.amount_cents), 0) AS ledger_balance
		FROM accounts a
		LEFT JOIN ledger_entries le ON le.account_id = a.id
		GROUP BY a.id, a.balance_cents
		ORDER BY a.id
	`)
	if err != nil {
		return Report{}, fmt.Errorf("query account balances: %w", err)
	}

	for rows.Next() {
		var accountID int64
		var actualBalance int64
		var ledgerBalance int64

		if err := rows.Scan(&accountID, &actualBalance, &ledgerBalance); err != nil {
			rows.Close()
			return Report{}, fmt.Errorf("scan account balance: %w", err)
		}

		report.AccountsChecked++

		if actualBalance != ledgerBalance {
			report.Discrepancies = append(report.Discrepancies, Discrepancy{
				Type:          "account_balance_mismatch",
				AccountID:     &accountID,
				ExpectedCents: ledgerBalance,
				ActualCents:   actualBalance,
			})
		}
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return Report{}, fmt.Errorf("iterate account balances: %w", err)
	}
	rows.Close()

	transactionRows, err := tx.Query(ctx, `
	SELECT
		t.id,
		COALESCE(SUM(le.amount_cents), 0) AS ledger_sum
	FROM transactions t
	LEFT JOIN ledger_entries le ON le.transaction_id = t.id
	GROUP BY t.id
	ORDER BY t.id
`)
	if err != nil {
		return Report{}, fmt.Errorf("query transaction balances: %w", err)
	}

	for transactionRows.Next() {
		var transactionID int64
		var ledgerSum int64

		if err := transactionRows.Scan(&transactionID, &ledgerSum); err != nil {
			transactionRows.Close()
			return Report{}, fmt.Errorf("scan transaction balance: %w", err)
		}

		report.TransactionsChecked++

		if ledgerSum != 0 {
			report.Discrepancies = append(report.Discrepancies, Discrepancy{
				Type:          "transaction_imbalance",
				TransactionID: &transactionID,
				ExpectedCents: 0,
				ActualCents:   ledgerSum,
			})
		}
	}

	if err := transactionRows.Err(); err != nil {
		transactionRows.Close()
		return Report{}, fmt.Errorf("iterate transaction balances: %w", err)
	}
	transactionRows.Close()

	if len(report.Discrepancies) > 0 {
		report.Status = "inconsistent"
	}

	if err := tx.Commit(ctx); err != nil {
		return Report{}, fmt.Errorf("commit reconciliation: %w", err)
	}

	return report, nil
}
