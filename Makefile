.PHONY: run down test psql

run:
	docker compose up --build

down:
	docker compose down

psql:
	docker compose exec postgres psql -U devices

test:
	go test -v -cover -count=1 ./...
