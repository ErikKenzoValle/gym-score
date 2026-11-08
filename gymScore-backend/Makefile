# Atalhos do backend. Integração: exige TEST_DB_DSN apontando para um MySQL 8 DESCARTÁVEL, ex.:
#   docker run -d --name gymscore-it -e MYSQL_ROOT_PASSWORD=$$(openssl rand -hex 16) -p 127.0.0.1:3307:3306 mysql:8
#   TEST_DB_DSN='root:SENHA@tcp(127.0.0.1:3307)/?parseTime=True&loc=Local' make test-integration
.PHONY: build vet test test-integration fmt vuln ci

build:
	go build ./...

vet:
	go vet ./...
	go vet -tags integration ./...

test:
	go test -race -count=1 ./...

test-integration:
	@test -n "$(TEST_DB_DSN)" || { echo "defina TEST_DB_DSN (MySQL 8 descartável)"; exit 1; }
	go test -tags integration -count=1 -timeout 60m ./internal/integration/...

fmt:
	gofmt -w .

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

ci: build vet test test-integration vuln
