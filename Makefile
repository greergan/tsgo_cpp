TSGO_REPO := ../typescript-go
TSGO_BRANCH := typescript/v7.0.2
TSGO_VERSION := $(notdir $(TSGO_BRANCH))
DEB_NAME := libslim-tsgo
DEB_VERSION := $(subst v,,$(TSGO_VERSION))
DEB_ARCH := amd64
DEB_DIR := dist/$(DEB_NAME)_$(DEB_VERSION)_$(DEB_ARCH)

all: slim_tsc_stub

$(TSGO_REPO):
	git clone --branch $(TSGO_BRANCH) --depth 1 https://github.com/microsoft/typescript-go.git $(TSGO_REPO)

libtsgo.a: $(TSGO_REPO)
	cp tsgo_bridge.go $(TSGO_REPO)/
#	cp -r ../typescript-bridge/types types
#	cp -r lib $(TSGO_REPO)/lib
	cp -r lib $(TSGO_REPO)/lib
	go mod tidy
	cd $(TSGO_REPO) && go build -buildmode=c-archive -o libslim_tsgo.a tsgo_bridge.go
	cp $(TSGO_REPO)/libslim_tsgo.a .
	cp $(TSGO_REPO)/libslim_tsgo.h .

slim_tsc_stub: clean libtsgo.a
	g++ -o slim_tsc_stub main.cpp libslim_tsgo.a -lpthread -ldl
	./slim_tsc_stub

deb: libtsgo.a
	mkdir -p $(DEB_DIR)/DEBIAN
	mkdir -p $(DEB_DIR)/usr/lib
	mkdir -p $(DEB_DIR)/usr/include
	cp libslim_tsgo.a $(DEB_DIR)/usr/lib/
	cp libslim_tsgo.h $(DEB_DIR)/usr/include/
	printf 'Package: $(DEB_NAME)\nVersion: $(DEB_VERSION)\nSection: libs\nPriority: optional\nArchitecture: $(DEB_ARCH)\nMaintainer: you\nDescription: TypeScript-Go C archive and header\n' > $(DEB_DIR)/DEBIAN/control
	dpkg-deb --build $(DEB_DIR)

clean:
	rm -rf libslim_tsgo.a libslim_tsgo.h slim_tsc_stub dist
	rm -f $(TSGO_REPO)/tsgo_bridge.go $(TSGO_REPO)/libslim_tsgo.h $(TSGO_REPO)/libslim_tsgo.a
	rm -rf $(TSGO_REPO)/lib
	clear
