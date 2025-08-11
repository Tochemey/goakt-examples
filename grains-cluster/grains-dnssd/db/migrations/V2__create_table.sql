CREATE TABLE sample.accounts(
	account_id VARCHAR(255) NOT NULL,
	account_balance NUMERIC(19, 2) NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	
	PRIMARY KEY (account_id)
);