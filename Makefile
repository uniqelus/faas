SERVICES := $(shell ls cmd)

BINARIES_GOOS ?= linux
BINARIES_GOARCH ?= amd64
BINARIES_CGO_ENABLED ?= 0

PROTO_DIR ?= api/proto

binaries: $(addsuffix -binary, $(SERVICES))
%-binary:
	@mkdir -p bin
	CGO_ENABLED=$(BINARIES_CGO_ENABLED) GOOS=$(BINARIES_GOOS) GOARCH=$(BINARIES_GOARCH) go build $(FLAGS) -o bin/$* ./cmd/$*

clean:
	rm -f $(addprefix bin/,$(SERVICES))

lint:
	golangci-lint run $(if $(FIX),,--fix) ./...

test: lint
	@if [ "$(INTEGRATION)" = "1" ]; then \
		$(MAKE) binaries; \
		go test -tags=integration -v -count=1 $(if $(RACE),-race,) ./tests/integration/...; \
	else \
		go test $(if $(RACE),-race,) ./...; \
	fi

proto-gen:
	$(MAKE) -C $(PROTO_DIR) generate

proto-lint:
	$(MAKE) -C $(PROTO_DIR) lint

proto-breaking:
	$(MAKE) -C $(PROTO_DIR) breaking

.PHONY: binaries clean lint test proto-gen proto-lint proto-breaking

