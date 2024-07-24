app_name	:= bngblasterctrl
app_ver		:= $(shell git describe --abbrev=0 --tags)

LDFLAGS		:= -X 'main.Version=$(app_ver)' -s -w $(LDFLAGS)

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