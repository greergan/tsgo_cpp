TSGO_REPO := ../typescript-go
TSGO_BRANCH := typescript/v7.0.2
TSGO_VERSION := $(notdir $(TSGO_BRANCH))

PREFIX         := /usr
INCLUDEDIR     := $(PREFIX)/include
LIBDIR_DEB     := $(PREFIX)/lib/$(shell dpkg-architecture -qDEB_HOST_MULTIARCH 2>/dev/null || echo x86_64-linux-gnu)
LIBDIR_RPM     := $(PREFIX)/lib64
PKG_NAME       := libtsgo
PKG_MAINTAINER := $(shell git config user.name) <$(shell git config user.email)>

PKG_VERSION := $(shell git describe --tags --exact-match 2>/dev/null | sed 's/^v//')

UNAME_M := $(shell uname -m)
ifeq ($(UNAME_M),x86_64)
  PKG_ARCH_DEB := amd64
  PKG_ARCH_RPM := x86_64
else ifeq ($(UNAME_M),aarch64)
  PKG_ARCH_DEB := arm64
  PKG_ARCH_RPM := aarch64
else
  PKG_ARCH_DEB := $(UNAME_M)
  PKG_ARCH_RPM := $(UNAME_M)
endif

OS_FAMILY := $(shell \
  . /etc/os-release 2>/dev/null && \
  echo "$$ID $$ID_LIKE" | grep -qE 'debian|ubuntu' && echo deb || echo rpm)

.PHONY: all fetch-tsgo build test test-cpp test-c package install clean

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
	@if [ -n "$${TSGO_LIB_DIR}" ] && [ ! -d "$${TSGO_LIB_DIR}" ]; then \
		echo "ERROR: TSGO_LIB_DIR '$${TSGO_LIB_DIR}' does not exist."; \
		exit 1; \
	fi
	@if [ -f libtsgo.a ] && [ -f libtsgo.h ] && \
		[ -f .lib_dir_stamp ] && [ "$${TSGO_LIB_DIR:-lib}" = "$$(cat .lib_dir_stamp)" ] && \
		diff -q libtsgo.go $(TSGO_REPO)/libtsgo.go > /dev/null 2>&1 && \
		diff -q $(TSGO_REPO)/libtsgo.a libtsgo.a > /dev/null 2>&1 && \
		diff -q $(TSGO_REPO)/libtsgo.h libtsgo.h > /dev/null 2>&1 && \
		diff -rq "$${TSGO_LIB_DIR:-lib}" $(TSGO_REPO)/lib > /dev/null 2>&1; then \
		echo "==> libtsgo.a and .h up to date, skipping Go build."; \
	else \
		rm -rf $(TSGO_REPO)/lib && cp -r "$${TSGO_LIB_DIR:-lib}" $(TSGO_REPO)/lib && \
		echo "$${TSGO_LIB_DIR:-lib}" > .lib_dir_stamp && \
		cp libtsgo.go $(TSGO_REPO)/ && \
		cd $(TSGO_REPO) && go build -buildmode=c-archive -o libtsgo.a libtsgo.go && \
		cp libtsgo.a $(CURDIR)/ && \
		cp libtsgo.h $(CURDIR)/; \
	fi
	go mod tidy

test: test-cpp test-c

test-cpp: build
	g++ -std=c++23 -o test_cpp test.cpp libtsgo.a -lpthread -ldl
	./test_cpp

test-c: build
	gcc -o test_c test.c libtsgo.a -lpthread -ldl
	./test_c

package: build
	@if [ -z "$(PKG_VERSION)" ]; then \
		echo "ERROR: No git tag found on current commit. Tag the repo before packaging."; \
		exit 1; \
	fi
	@echo "==> Packaging $(PKG_NAME) $(PKG_VERSION)..."

ifeq ($(OS_FAMILY),deb)
	@echo "==> Staging .deb..."
	@rm -rf dist/deb
	@mkdir -p dist/deb/DEBIAN
	@mkdir -p dist/deb$(LIBDIR_DEB)
	@mkdir -p dist/deb$(INCLUDEDIR)
	@cp libtsgo.a  dist/deb$(LIBDIR_DEB)/libtsgo.a
	@cp libtsgo.h  dist/deb$(INCLUDEDIR)/libtsgo.h
	@cp tsgo.h     dist/deb$(INCLUDEDIR)/tsgo.h
	@printf "Package: $(PKG_NAME)\nVersion: $(PKG_VERSION)\nArchitecture: $(PKG_ARCH_DEB)\nMaintainer: $(PKG_MAINTAINER)\nDescription: TypeScript Go static library\n" \
		> dist/deb/DEBIAN/control
	@dpkg-deb --build dist/deb dist/$(PKG_NAME)_$(PKG_VERSION)_$(PKG_ARCH_DEB).deb
	@echo "==> Created dist/$(PKG_NAME)_$(PKG_VERSION)_$(PKG_ARCH_DEB).deb"
else ifeq ($(OS_FAMILY),rpm)
	@echo "==> Staging .rpm..."
	@rm -rf dist/rpm dist/rpmbuild
	@mkdir -p dist/rpm$(LIBDIR_RPM)
	@mkdir -p dist/rpm$(INCLUDEDIR)
	@cp libtsgo.a  dist/rpm$(LIBDIR_RPM)/libtsgo.a
	@cp libtsgo.h  dist/rpm$(INCLUDEDIR)/libtsgo.h
	@cp tsgo.h     dist/rpm$(INCLUDEDIR)/tsgo.h
	@mkdir -p dist/rpmbuild/BUILD
	@mkdir -p dist/rpmbuild/RPMS
	@mkdir -p dist/rpmbuild/SOURCES
	@mkdir -p dist/rpmbuild/SPECS
	@mkdir -p dist/rpmbuild/SRPMS
	@printf "Name:           $(PKG_NAME)\n\
Version:        $(PKG_VERSION)\n\
Release:        1\n\
Summary:        TypeScript Go static library\n\
License:        Apache-2.0\n\
BuildArch:      $(PKG_ARCH_RPM)\n\
\n\
%%description\n\
TypeScript Go static library\n\
\n\
%%install\n\
mkdir -p %%{buildroot}$(LIBDIR_RPM)\n\
mkdir -p %%{buildroot}$(INCLUDEDIR)\n\
cp $(CURDIR)/libtsgo.a  %%{buildroot}$(LIBDIR_RPM)/libtsgo.a\n\
cp $(CURDIR)/libtsgo.h  %%{buildroot}$(INCLUDEDIR)/libtsgo.h\n\
cp $(CURDIR)/tsgo.h     %%{buildroot}$(INCLUDEDIR)/tsgo.h\n\
\n\
%%files\n\
$(LIBDIR_RPM)/libtsgo.a\n\
$(INCLUDEDIR)/libtsgo.h\n\
$(INCLUDEDIR)/tsgo.h\n" \
		> dist/rpmbuild/SPECS/$(PKG_NAME).spec
	@rpmbuild -bb --quiet \
		--define "_topdir $(CURDIR)/dist/rpmbuild" \
		--define "_rpmdir $(CURDIR)/dist" \
		dist/rpmbuild/SPECS/$(PKG_NAME).spec
	@mv dist/$(PKG_ARCH_RPM)/$(PKG_NAME)-$(PKG_VERSION)-1.$(PKG_ARCH_RPM).rpm dist/
	@rm -rf dist/$(PKG_ARCH_RPM)
	@echo "==> Created dist/$(PKG_NAME)-$(PKG_VERSION)-1.$(PKG_ARCH_RPM).rpm"
else
	$(error Unsupported OS family '$(OS_FAMILY)'. Cannot package.)
endif

install: package
	@echo "==> Installing $(PKG_NAME) $(PKG_VERSION) ($(OS_FAMILY))..."
	@if [ "$(OS_FAMILY)" = "deb" ]; then \
		dpkg -i dist/$(PKG_NAME)_$(PKG_VERSION)_$(PKG_ARCH_DEB).deb; \
	else \
		rpm -i dist/$(PKG_NAME)-$(PKG_VERSION)-1.$(PKG_ARCH_RPM).rpm; \
	fi

clean:
	rm -rf libtsgo.a libtsgo.h test_cpp test_c dist .lib_dir_stamp
