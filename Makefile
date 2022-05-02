app_name	:= bngblasterctrl
app_ver		?= 0.1.0-dev

# Just in case there is an extra space at the end of the line.
app_name	:= $(shell echo "$(app_name)" | head -n 1 | awk '{printf("%s", $$1);}')
app_ver		:= $(shell echo "$(app_ver)" | head -n 1 | awk '{printf("%s", $$1);}')

LDFLAGS		:= -X "main.VERSION=$(app_ver)" -s -w $(LDFLAGS)

OS := $(if $(GOOS),$(GOOS),$(shell go env GOOS))
ARCH := $(if $(GOARCH),$(GOARCH),$(shell go env GOARCH))

fumpt:
	gofumpt -l -w .

test:
	go test -cover ./...

lint:
	golangci-lint run

gci:
	gci write -s Standard -s Default -s "Prefix(github.com/rtbrick)" .

build-%:
	@echo	"[run] build-OS_ARCH"
	@$(MAKE) build                        \
	    --no-print-directory              \
	    GOOS=$(firstword $(subst _, ,$*)) \
	    GOARCH=$(lastword $(subst _, ,$*))

BUILD_DIR := bin/$(OS)_$(ARCH)

build:
	@echo build $(OS)_$(ARCH) $(app_ver)
	@mkdir -p $(BUILD_DIR)
	env GOOS=$(OS) GOARCH=$(ARCH) go build -o $(BUILD_DIR)/$(app_name) -ldflags '$(LDFLAGS)' ./cmd/$(app_name)/

.PHONY: clean test
clean:
	@echo "[run] clean"
	@- rm -rf bin