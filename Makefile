.PHONY: db test emulator backend web

db:
	docker compose up -d --wait postgres

test: db
	go test -p 1 ./...

emulator:
	go tool air -c emulator/.air.toml

backend:
	go tool air -c backend/.air.toml

web:
	cd web && npm run dev
