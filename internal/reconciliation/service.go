package reconciliation

import "context"

type Discrepancy struct {
	Type          string `json:"type"`
	AccountID     *int64 `json:"account_id,omitempty"`
	TransactionID *int64 `json:"transaction_id,omitempty"`
	ExpectedCents int64  `json:"expected_cents"`
	ActualCents   int64  `json:"actual_cents"`
}

type Report struct {
	Status              string        `json:"status"`
	AccountsChecked     int64         `json:"accounts_checked"`
	TransactionsChecked int64         `json:"transactions_checked"`
	Discrepancies       []Discrepancy `json:"discrepancies"`
}

type Service interface {
	Reconcile(ctx context.Context) (Report, error)
}
