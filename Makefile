TSGO_REPO := ../typescript-go
TSGO_BRANCH := typescript/v7.0.2
TSGO_VERSION := $(notdir $(TSGO_BRANCH))
.PHONY: all fetch-tsgo build test test-cpp test-c clean

all: build

fetch-tsgo:
	@echo "==> Fetching typescript-go..."
	@if [ -d $(TSGO_REPO) ]; then \
		git -C $(TSGO_REPO) fetch --depth 1 origin $(TSGO_BRANCH) && \
		git -C $(TSGO_REPO) checkout FETCH_HEAD; \
	else \
		git clone --branch $(TSGO_BRANCH) --depth 1 https://github.com/microsoft/typescript-go.git $(TSGO_REPO); \
		git -C $(TSGO_REPO) fetch --tags; \
	fi

build: fetch-tsgo
	@echo "==> Building libtsgo..."
	@rm -rf $(TSGO_REPO)/lib && cp -r lib $(TSGO_REPO)/lib
	go mod tidy
	@if [ -f libtsgo.a ] && [ -f libtsgo.h ] && \
		diff -q libtsgo.go $(TSGO_REPO)/libtsgo.go > /dev/null 2>&1 && \
		diff -q $(TSGO_REPO)/libtsgo.a libtsgo.a > /dev/null 2>&1 && \
		diff -q $(TSGO_REPO)/libtsgo.h libtsgo.h > /dev/null 2>&1; then \
		echo "==> libtsgo.a and .h up to date, skipping Go build."; \
	else \
		cp libtsgo.go $(TSGO_REPO)/ && \
		cd $(TSGO_REPO) && go build -buildmode=c-archive -o libtsgo.a libtsgo.go && \
		cp libtsgo.a $(CURDIR)/ && \
		cp libtsgo.h $(CURDIR)/; \
	fi

test: test-cpp test-c

test-cpp: build
	g++ -std=c++23 -o test_cpp test.cpp libtsgo.a -lpthread -ldl
	./test_cpp

test-c: build
	gcc -o test_c test.c libtsgo.a -lpthread -ldl
	./test_c

clean:
	rm -rf libtsgo.a libtsgo.h test_cpp test_c dist
