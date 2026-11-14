.PHONY: run down test test-container psql

run:
	docker compose up --build

down:
	docker compose down

psql:
	docker compose exec postgres psql -U devices

# Ignores Postgres integration tests, because there is no test database.
test:
	go test -v -cover -count=1 ./...

test-container:
	docker compose --profile test run --rm test; \
	status=$$?; \
	docker compose --profile test rm --stop --force postgres-test; \
	exit $$status
