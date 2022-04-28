app_name	:= bngblasterctrl
app_ver		?= 0.1.0-dev

# Just in case there is an extra space at the end of the line.
app_name	:= $(shell echo "$(app_name)" | head -n 1 | awk '{printf("%s", $$1);}')
app_ver		:= $(shell echo "$(app_ver)" | head -n 1 | awk '{printf("%s", $$1);}')

LDFLAGS		:= -X "main.VERSION=$(app_ver)" -s -w $(LDFLAGS)

OS := $(if $(GOOS),$(GOOS),$(shell go env GOOS))
ARCH := $(if $(GOARCH),$(GOARCH),$(shell go env GOARCH))

all: gocmds

linters:
	@echo "[run] linters"
	@# We do this instead of a simple `go fmt ...` because (at least in the
	@# beginning) it's better too see the changes than blindly run it.
	@echo "gofmt -l -e ."; \
		fmt_out=`gofmt -l -e pkg cmd` || exit 1; \
		[ -z "$$fmt_out" ] || { \
			echo "$$fmt_out"; \
			echo "#"; \
			echo "# If you want a quick fix just run: go fmt ."; \
			echo "#"; \
			exit 1; \
		};
	@which golint > /dev/null || { \
		echo "#"; \
		echo "# Either you don't have golint installed or it's not accessible."; \
		echo "#"; \
		echo "# Make sure you have \$$GOPATH set up correctly and that \$$GOPATH/bin is included in your \$$PATH,"; \
		echo "# see https://golang.org/doc/code.html#GOPATH & https://github.com/golang/go/wiki/GOPATH ."; \
		echo "#"; \
		echo "# After that run: go get -u golang.org/x/lint/golint"; \
		echo "# see https://github.com/golang/lint ."; \
		echo "#"; \
		exit 1; \
	};
	golint -set_exit_status $(shell go list ./... | grep -v /vendor/)
	go vet $(shell go list ./... | grep -v /vendor/)

test:
	@echo "[run] tests"
	@mkdir -p $(BUILD_DIR)
	go test -coverprofile=./bin/cover.out -cover $(shell go list ./... | grep -v /cmd/)
	go test -coverprofile=./bin/cover.out -json  $(shell go list ./... | grep -v /cmd/) > ./bin/testreport.json

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
	#env GOOS=$(OS) GOARCH=$(ARCH) go build -o $(BUILD_DIR)/bngblaster -ldflags '$(LDFLAGS)' ./cmd/bngblaster/

gocmds: linters test build-linux_amd64

.PHONY: clean test
clean:
	@echo "[run] clean"
	@- rm -rf bin

.PHONY: install
install:
	install -o root -g root -m 755 ./bin/linux_amd64/bngblasterctrl /usr/local/bin/bngblasterctrl
