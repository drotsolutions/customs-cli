LINT_VERSION := 1.57.2

.PHONY: build
build:
	go build -mod=vendor -o customs .

.PHONY: lint
lint:
	gofmt -s -w ./main.go ./client.go
	stat ./bin/golangci-lint > /dev/null && ./bin/golangci-lint --version | grep -q $(LINT_VERSION) || \
    	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v$(LINT_VERSION)
	./bin/golangci-lint run --timeout 3m

.PHONY: mod
mod:
	go mod tidy
	go mod vendor
