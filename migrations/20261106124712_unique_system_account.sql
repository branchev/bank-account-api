
-- +goose Up

CREATE UNIQUE INDEX idx_accounts_single_system
ON accounts (account_type)
WHERE account_type = 'system';


-- +goose Down

DROP INDEX idx_accounts_single_system;
