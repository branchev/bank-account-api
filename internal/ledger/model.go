package ledger

type Entry struct {
	ID            int64
	TransactionID int64
	AccountID     int64
	AmountCents   int64
}
