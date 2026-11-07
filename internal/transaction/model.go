package transaction

// Type represents the kind of financial operation.
type Type string

const (
	TypeAccountCreation Type = "account_creation"
	TypeDeposit         Type = "deposit"
	TypeTransfer        Type = "transfer"
)

// Transaction represents a financial operation recorded in the database.
type Transaction struct {
	ID             int64
	Type           Type
	IdempotencyKey string
	RequestHash    string
	ResponseStatus *int
	ResponseBody   []byte
}
