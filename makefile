main:
	go run main.go
sqlc:
	sqlc generate
test:
	go test -v -cover -short ./...

	
.PHONY:sqlc main test