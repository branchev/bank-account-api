package account

type Account struct {
	ID           int64  `json:"id"`
	Currency     string `json:"currency"`
	BalanceCents int64  `json:"balance_cents"`
}
