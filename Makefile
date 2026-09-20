DB_URL=postgresql://nikhil:Mahi1234@localhost:5432/bank_ledger?sslmode=disable&TimeZone=Asia/Kolkata
MIGRATION_PATH=db/migration

.PHONY: migrateup migratedown run test

migrateup:
	migrate -path $(MIGRATION_PATH) -database "$(DB_URL)" -verbose up

migratedown:
	migrate -path $(MIGRATION_PATH) -database "$(DB_URL)" -verbose down

run:
	go run cmd/api/main.go

test:
	go test -v -cover ./...
