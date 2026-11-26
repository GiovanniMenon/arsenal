BINDIR  ?= $(HOME)/.local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/ynn3m/arsenal/internal/cli.Version=$(VERSION)
CONFIG  ?= $${XDG_CONFIG_HOME:-$(HOME)/.config}/arsenal/config.yml

ifeq ($(V),1)
Q :=
else
Q := @
endif

.PHONY: build install uninstall test vet fmt clean

build:
	$(Q)printf 'Building ars (%s)...' "$(VERSION)"; \
	if CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/ars ./cmd/ars; then echo " done"; else echo " failed"; exit 1; fi

install: build
	$(Q)install -d "$(BINDIR)"
	$(Q)install -m 0755 bin/ars "$(BINDIR)/ars"
	$(Q)ln -sf ars "$(BINDIR)/arsenal"
	$(Q)echo "Installed ars and arsenal in $(BINDIR)"
	$(Q)case ":$$PATH:" in *":$(BINDIR):"*) ;; *) echo "Note: $(BINDIR) is not in your PATH; 'ars setup' adds it to your shell rc file." ;; esac
	$(Q)if [ -f "$(CONFIG)" ]; then echo "Restart the terminal if ars was not in your PATH"; else echo "Next: ars setup"; fi

uninstall:
	$(Q)rm -f "$(BINDIR)/ars" "$(BINDIR)/arsenal"
	$(Q)echo "Removed ars and arsenal from $(BINDIR) (your config in ~/.config/arsenal is kept)"

test:
	$(Q)go test ./...

vet:
	$(Q)go vet ./...

fmt:
	$(Q)gofmt -w .

clean:
	$(Q)rm -rf bin dist
