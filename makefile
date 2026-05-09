DB_URL=agentgo.db

main:
	go run main.go

sqlc:
	sqlc generate

test:
	go test -v -cover -short ./...

migrateup:
	migrate -path internal/db/migration -database "$(DB_URL)" -verbose up

migrateup1:
	migrate -path internal/db/migration -database "$(DB_URL)" -verbose up 1

migratedown:
	migrate -path internal/db/migration -database "$(DB_URL)" -verbose down

migratedown1:
	migrate -path internal/db/migration -database "$(DB_URL)" -verbose down 1

new_migration:
	migrate create -ext sql -dir internal/db/migrationn -seq $(name)

.PHONY:sqlc main test migrateup