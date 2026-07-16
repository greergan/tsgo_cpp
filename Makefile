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
	cp tsgo_cpp.go $(TSGO_REPO)/
	cp -r lib $(TSGO_REPO)/lib
	go mod tidy
	@if [ -f libtsgo_cpp.a ] && [ -f libtsgo_cpp.h ] && \
		diff -q $(TSGO_REPO)/libtsgo_cpp.a libtsgo_cpp.a > /dev/null 2>&1 && \
		diff -q $(TSGO_REPO)/libtsgo_cpp.h libtsgo_cpp.h > /dev/null 2>&1; then \
		echo "==> libtsgo_cpp.a and .h up to date, skipping Go build."; \
	else \
	    cd $(TSGO_REPO) && go build -buildmode=c-archive -o libtsgo_cpp.a tsgo_cpp.go && \
		cp libtsgo_cpp.a $(CURDIR)/ && \
		cp libtsgo_cpp.h $(CURDIR)/; \
	fi
test: test-cpp test-c
test-cpp: build
	g++ -o test_cpp test.cpp libtsgo_cpp.a -lpthread -ldl
	./test_cpp
test-c: build
	gcc -o test_c test.c libtsgo_cpp.a -lpthread -ldl
	./test_c
clean:
	rm -rf libtsgo_cpp.a libtsgo_cpp.h test_cpp test_c dist
	rm -f $(TSGO_REPO)/tsgo_cpp.go $(TSGO_REPO)/libtsgo_cpp.h $(TSGO_REPO)/libtsgo_cpp.a
	rm -rf $(TSGO_REPO)/lib
