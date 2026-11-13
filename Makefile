# Common tasks. On Windows, run these from Git Bash.
DB ?= postgres://ats:ats@localhost:5432/ats?sslmode=disable
TEST_DB ?= postgres://ats:ats@localhost:5432/ats_test?sslmode=disable

.PHONY: run test test-db build fmt vet up

run:            ## Run the server locally (reads .env if you export it first)
	DATABASE_URL="$(DB)" go run ./cmd/server

test:           ## Unit tests (no database needed)
	go test ./...

test-db:        ## Unit + database integration tests
	TEST_DATABASE_URL="$(TEST_DB)" go test ./...

build:
	go build -o bin/server ./cmd/server

fmt:
	gofmt -s -w .

vet:
	go vet ./...

up:             ## Postgres + app in Docker (demo AI)
	docker compose up --build
