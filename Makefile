.PHONY: db test emulator backend web up

# local development: each target in its own terminal
db:
	docker compose up -d --wait postgres

emulator:
	go tool air -c emulator/.air.toml

backend:
	go tool air -c backend/.air.toml

web:
	cd web && npm run dev

# full run in Docker: https://localhost
up:
	docker compose up --build

test: db
	go test -p 1 ./...
