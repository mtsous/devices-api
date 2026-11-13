.PHONY: run down test test-container psql

run:
	docker compose up --build

down:
	docker compose down

psql:
	docker compose exec postgres psql -U devices

test:
	go test -v -cover -count=1 ./...

# Runs all tests, including Postgres integration tests, inside containers.
# The test database is removed afterwards; the exit code is the tests' result.
test-container:
	docker compose --profile test run --rm test; \
	status=$$?; \
	docker compose --profile test rm --stop --force postgres-test; \
	exit $$status
