ALTER TABLE accounts
    ADD CONSTRAINT fk_accounts_owner
    FOREIGN KEY (owner)
    REFERENCES users(username)
    ON DELETE CASCADE;
