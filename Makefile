DATABASE_URL ?= $(shell grep '^DATABASE_URL=' .env 2>/dev/null | cut -d '=' -f 2-)
MIGRATION_PATH=db/migration

.PHONY: migrateup migratedown run test

migrateup:
	migrate -path $(MIGRATION_PATH) -database "$(DATABASE_URL)" -verbose up

migratedown:
	migrate -path $(MIGRATION_PATH) -database "$(DATABASE_URL)" -verbose down

run:
	go run cmd/api/main.go

test:
	go test -v -cover ./...
