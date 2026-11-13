.PHONY: run down test

run:
	docker compose up --build

down:
	docker compose down

test:
	go test -v -cover -count=1 ./...
