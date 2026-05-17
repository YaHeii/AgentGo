DB_URL=agentgo.db

main:
	go run main.go

sqlc:
	sqlc generate

test:
	go test -v -cover -short ./...

migrateup:
	go run ./cmd/migrate -db "$(DB_URL)" up

migrateup1:
	go run ./cmd/migrate -db "$(DB_URL)" up 1

migratedown:
	go run ./cmd/migrate -db "$(DB_URL)" down

migratedown1:
	go run ./cmd/migrate -db "$(DB_URL)" down 1

new_migration:
	@echo "Create internal/db/migrations/<seq>_<name>.up.sql and .down.sql manually"

.PHONY:sqlc main test migrateup
