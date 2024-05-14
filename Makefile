PKGS = $(shell go list ./... | grep -v /examples)

.PHONY: all
all: clean test coverage

.PHONY: test
test: fmt vet
	go test -coverprofile coverage.out $(PKGS)

.PHONY: coverage
coverage:
	go tool cover -func=coverage.out -o coverage.txt
	go tool cover -html=coverage.out -o coverage.html
	@cat coverage.txt
	@echo "Run 'open coverage.html' to view coverage report."

.PHONY: fmt
fmt:
	go fmt go.nanomsg.org/mangos/v3/...

.PHONY: vet
vet:
	go vet go.nanomsg.org/mangos/v3/...

.PHONY: clean
clean:
	go mod tidy
	go clean ./...
	rm -f coverage.*