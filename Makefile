.PHONY: help build run test test-v test-cover lint fmt tidy clean install-tools

BINARY := ralph
PKG := ./...

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## 编译二进制到 bin/
	@mkdir -p bin
	go build -o bin/$(BINARY) ./cmd/$(BINARY)

run: build ## 编译并运行
	./bin/$(BINARY)

test: ## 跑测试
	go test -race -count=1 $(PKG)

test-v: ## 跑测试（详细）
	go test -race -count=1 -v $(PKG)

test-cover: ## 跑测试 + 覆盖率
	go test -race -count=1 -coverprofile=coverage.txt -covermode=atomic $(PKG)
	go tool cover -html=coverage.txt -o coverage.html
	@echo "覆盖率报告：coverage.html"

lint: ## 跑 golangci-lint
	golangci-lint run

fmt: ## 格式化代码
	gofmt -s -w .
	goimports -w .

tidy: ## 整理依赖
	go mod tidy

clean: ## 清理产物
	rm -rf bin/ dist/ coverage.txt coverage.html

install-tools: ## 安装开发工具
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
