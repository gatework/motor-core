GO ?= go

# Shared checks run the full race suite. External integration fixtures are
# explicitly selected through environment variables documented by their tests.

.PHONY: all build vet test test-race canary-test bench bench-compare soak lint vuln tidy api-snapshot api-check ci

all: ci

build:
	$(GO) build ./...

test-race:
	$(GO) test -short -race -timeout 600s ./...

canary-test:
	scripts/canary-test.sh

# bench runs every benchmark once, briefly. It is a smoke test that the
# benchmarks still compile and run - not a measurement. For numbers worth
# comparing, raise -benchtime and use benchstat over several runs.
#
# Redis benchmarks skip unless REDIS_URL points at a server.
bench:
	$(GO) test -run '^$$' -bench . -benchtime 10x -benchmem ./...

# bench-compare measures this tree against a base revision on one machine and
# runs the results through benchstat. Timings are not gated in CI - see the
# script's header - so this is the deliberate check to run when a change is
# meant to affect performance.
#
#	make bench-compare                          # against origin/main
#	make bench-compare BASE=HEAD~1 BENCH=Cache  # narrower
BASE ?= origin/main
BENCH ?= .
BENCH_COUNT ?= 6
bench-compare:
	scripts/bench-compare.sh $(BASE) $(BENCH) $(BENCH_COUNT)

# soak runs the tests that need time: sustained load, resource reclamation and
# goroutine lifecycles. They are skipped by -short, which is what test and
# test-race use, so nothing else in this Makefile covers them.
#
# GOADMIN_SOAK sets how long each one runs, e.g. GOADMIN_SOAK=2m make soak.
soak:
	$(GO) test -race -timeout 1800s -run 'Soak|Leak|Stall|Reclaim|Release' ./...

api-snapshot:
	scripts/api-snapshot.sh

# Fails when the committed snapshot no longer matches the code, forcing API
# changes to surface in the pull request diff.
api-check: api-snapshot
	git diff --exit-code api/

ci: check

CHECK_EXTRA := build canary-test api-check language release-check
include standard.mk

# Limit compiler parallelism; retain all shared checks and full race tests.
export GOFLAGS := -mod=readonly -p=2

.PHONY: language
language:
	scripts/check-language.sh $(BASE)

.PHONY: release-check
release-check:
	python3 scripts/test_package.py
	python3 scripts/test_release.py
