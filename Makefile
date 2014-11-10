PACKAGE := github.com/travis-pro/worker-manager-service
SUBPACKAGES := \
	$(PACKAGE)/cmd/wm-server \
	$(PACKAGE)/cmd/wm-workers \
	$(PACKAGE)/lib \
	$(PACKAGE)/lib/db \
	$(PACKAGE)/lib/server \
	$(PACKAGE)/lib/server/jsonapi \
	$(PACKAGE)/lib/server/negroniraven \
	$(PACKAGE)/lib/workers

VERSION_VAR := $(PACKAGE)/lib.VersionString
VERSION_VALUE ?= $(shell git describe --always --dirty --tags 2>/dev/null)
REV_VAR := $(PACKAGE)/lib.RevisionString
REV_VALUE ?= $(shell git rev-parse --sq HEAD 2>/dev/null || echo "'???'")
GENERATED_VAR := $(PACKAGE)/lib.GeneratedString
GENERATED_VALUE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%S%z')

FIND ?= find
GO ?= go
DEPPY ?= deppy
GOPATH := $(shell echo $${GOPATH%%:*})
GOBUILD_LDFLAGS ?= -ldflags "\
	-X $(VERSION_VAR) '$(VERSION_VALUE)' \
	-X $(REV_VAR) $(REV_VALUE) \
	-X $(GENERATED_VAR) '$(GENERATED_VALUE)' \
"
GOBUILD_FLAGS ?= -x

PORT ?= 9839
export PORT

COVERPROFILES := \
	lib-coverage.coverprofile \
	lib-db-coverage.coverprofile \
	lib-server-coverage.coverprofile \
	lib-server-jsonapi-coverage.coverprofile \
	lib-server-negroniraven-coverage.coverprofile \
	lib-workers-coverage.coverprofile

%-coverage.coverprofile:
	$(GO) test -covermode=count -coverprofile=$@ \
		$(GOBUILD_LDFLAGS) $(PACKAGE)/$(subst -,/,$(subst -coverage.coverprofile,,$@))

.PHONY: all
all: clean deps test lintall

.PHONY: buildpack
buildpack:
	@$(MAKE) build \
		GOBUILD_FLAGS= \
		REV_VALUE="'$(shell git log -1 --format='%H')'" \
		VERSION_VALUE=heroku \
		| sed 's/^/       /'

.PHONY: test
test: build fmtpolice test-deps coverage.html

.PHONY: test-deps
test-deps:
	$(GO) test -i $(GOBUILD_LDFLAGS) $(PACKAGE) $(SUBPACKAGES)

# .PHONY: test-race
# test-race:
# 	$(GO) test -race $(GOBUILD_LDFLAGS) $(PACKAGE) $(SUBPACKAGES)

coverage.html: coverage.coverprofile
	$(GO) tool cover -html=$^ -o $@

coverage.coverprofile: $(COVERPROFILES)
	./bin/fold-coverprofiles $^ > $@
	$(GO) tool cover -func=$@

.PHONY: build
build:
	$(GO) install $(GOBUILD_FLAGS) $(GOBUILD_LDFLAGS) $(PACKAGE) $(SUBPACKAGES)

.PHONY: deps
deps:
	$(GO) get -t $(GOBUILD_FLAGS) $(GOBUILD_LDFLAGS) $(PACKAGE) $(SUBPACKAGES)

.PHONY: clean
clean:
	./bin/clean

.PHONY: annotations
annotations:
	@git grep -E '(TODO|FIXME|XXX):' | grep -v Makefile

.PHONY: save
save:
	$(DEPPY) save ./...

.PHONY: fmtpolice
fmtpolice:
	./bin/fmtpolice

.PHONY: lintall
lintall:
	./bin/lintall
