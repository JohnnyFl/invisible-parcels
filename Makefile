.PHONY: db test

db:
	docker compose up -d --wait postgres

test: db
	go test -p 1 ./...
