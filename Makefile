TSGO_REPO := ../typescript-go
TSGO_BRANCH := typescript/v7.0.2
TSGO_VERSION := $(notdir $(TSGO_BRANCH))

.PHONY: all fetch-tsgo build clean

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
	cd $(TSGO_REPO) && go build -buildmode=c-archive -o libtsgo_cpp.a tsgo_cpp.go
	cp $(TSGO_REPO)/libtsgo_cpp.a .
	cp $(TSGO_REPO)/libtsgo_cpp.h .

test: build
	g++ -o test test.cpp libtsgo_cpp.a -lpthread -ldl
	./test

clean:
	rm -rf libtsgo_cpp.a libtsgo_cpp.h test dist
	rm -f $(TSGO_REPO)/tsgo_cpp.go $(TSGO_REPO)/libtsgo_cpp.h $(TSGO_REPO)/libtsgo_cpp.a
	rm -rf $(TSGO_REPO)/lib
