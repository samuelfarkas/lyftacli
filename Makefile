GO ?= go
INSTALL ?= install

BINARY := lyfta
PACKAGE := ./cmd/lyfta
BUILD_DIR ?= bin
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
DESTDIR ?=
GOFLAGS ?=
LDFLAGS ?=

BUILD_OUTPUT := $(BUILD_DIR)/$(BINARY)
INSTALL_PATH := $(DESTDIR)$(BINDIR)/$(BINARY)

.PHONY: all build install install-built uninstall test vet verify path help

all: build

build:
	@mkdir -p "$(BUILD_DIR)"
	CGO_ENABLED=0 "$(GO)" build $(GOFLAGS) -trimpath $(if $(strip $(LDFLAGS)),-ldflags "$(LDFLAGS)") -o "$(BUILD_OUTPUT)" "$(PACKAGE)"

install: build
	@$(INSTALL) -d "$(DESTDIR)$(BINDIR)"
	@$(INSTALL) -m 0755 "$(BUILD_OUTPUT)" "$(INSTALL_PATH)"
	@printf 'Installed %s\n' "$(INSTALL_PATH)"

install-built:
	@test -x "$(BUILD_OUTPUT)" || { printf '%s\n' '$(BUILD_OUTPUT) is missing; run make build first' >&2; exit 1; }
	@$(INSTALL) -d "$(DESTDIR)$(BINDIR)"
	@$(INSTALL) -m 0755 "$(BUILD_OUTPUT)" "$(INSTALL_PATH)"
	@printf 'Installed %s\n' "$(INSTALL_PATH)"

uninstall:
	@rm -f "$(INSTALL_PATH)"
	@printf 'Removed %s\n' "$(INSTALL_PATH)"

test:
	"$(GO)" test ./...

vet:
	"$(GO)" vet ./...

verify: test vet

path:
	@printf 'export PATH="%s:$$PATH"\n' "$(BINDIR)"

help:
	@printf '%s\n' \
	  'make build             Build bin/lyfta from source' \
	  'make install           Build and install to ~/.local/bin' \
	  'make uninstall         Remove the installed binary' \
	  'make verify            Run tests and go vet' \
	  'make path              Print the PATH export command' \
	  '' \
	  'Override the destination with BINDIR=/your/bin or PREFIX=/your/prefix.'
