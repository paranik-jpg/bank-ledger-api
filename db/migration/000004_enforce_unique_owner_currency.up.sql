ALTER TABLE accounts
    ADD CONSTRAINT uq_accounts_owner_currency
    UNIQUE (owner, currency);
