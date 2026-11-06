
-- +goose Up

CREATE TABLE accounts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    account_type TEXT NOT NULL DEFAULT 'customer'
        CHECK (account_type IN ('customer', 'system')),

    currency CHAR(3) NOT NULL DEFAULT 'EUR'
        CHECK (currency = 'EUR'),

    balance_cents BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT customer_balance_nonnegative
        CHECK (
            account_type = 'system'
            OR balance_cents >= 0
        )
);


CREATE TABLE transactions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    transaction_type TEXT NOT NULL
        CHECK (
            transaction_type IN (
                'account_creation',
                'deposit',
                'transfer'
            )
        ),

    idempotency_key TEXT NOT NULL UNIQUE,
    request_hash TEXT NOT NULL,

    response_status INTEGER
        CHECK (response_status BETWEEN 200 AND 299),

    response_body JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT transaction_response_complete
        CHECK (
            (response_status IS NULL AND response_body IS NULL)
            OR
            (response_status IS NOT NULL AND response_body IS NOT NULL)
        )
);


CREATE TABLE ledger_entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    transaction_id BIGINT NOT NULL
        REFERENCES transactions(id),

    account_id BIGINT NOT NULL
        REFERENCES accounts(id),

    amount_cents BIGINT NOT NULL
        CHECK (amount_cents <> 0),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


CREATE INDEX idx_ledger_entries_account
    ON ledger_entries(account_id);

CREATE INDEX idx_ledger_entries_transaction
    ON ledger_entries(transaction_id);


-- Prevent modifications to posted ledger entries.
-- Corrections must be represented by new financial transactions.

-- +goose StatementBegin
CREATE FUNCTION prevent_ledger_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Ledger entries are immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER ledger_entries_immutable
BEFORE UPDATE OR DELETE ON ledger_entries
FOR EACH ROW
EXECUTE FUNCTION prevent_ledger_modification();


-- Dedicated internal funding account.
-- Used as the counterparty for deposits and initial balances.

INSERT INTO accounts (
    account_type,
    currency,
    balance_cents
)
VALUES (
    'system',
    'EUR',
    0
);


-- +goose Down

DROP TABLE ledger_entries;
DROP TABLE transactions;
DROP TABLE accounts;

DROP FUNCTION prevent_ledger_modification();
