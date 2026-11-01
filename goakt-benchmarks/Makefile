# Benchmarks comparing GoAkt with other Go actor frameworks.
#
# Every framework lives in its own package and runs the same scenarios under the
# same benchmark names, so the results can be compared side by side with
# benchstat (declared as a tool in go.mod).

.DEFAULT_GOAL := help

FRAMEWORKS := goakt ergo protoactor hollywood
# Only these frameworks have grains (virtual actors).
GRAIN_FRAMEWORKS := goakt protoactor

BENCH     ?= .
COUNT     ?= 10
BENCHTIME ?= 1s
CPU       ?=
RESULTS   ?= results

GO_TEST := go test -run '^$$' -bench '$(BENCH)' -benchmem -count $(COUNT) -benchtime $(BENCHTIME) $(if $(CPU),-cpu $(CPU))

.PHONY: help
help:
	@echo "GoAkt Benchmarks"
	@echo ""
	@echo "Targets:"
	@echo "  bench              Benchmark every framework and compare the results"
	@echo "  bench-<framework>  Benchmark one framework ($(FRAMEWORKS))"
	@echo "  compare            Compare the saved results with benchstat"
	@echo "  table              Print the saved actor results as Markdown comparison tables"
	@echo "  table-grains       Print the saved grain results as Markdown comparison tables"
	@echo "  smoke              Run every benchmark once, to check they work"
	@echo "  vendor             go mod tidy && go mod vendor"
	@echo "  clean              Remove the saved results"
	@echo ""
	@echo "Variables:"
	@echo "  BENCH=$(BENCH) COUNT=$(COUNT) BENCHTIME=$(BENCHTIME) CPU=$(CPU)"

.PHONY: bench
bench: $(addprefix bench-,$(FRAMEWORKS))
	@$(MAKE) --no-print-directory compare

.PHONY: bench-%
bench-%:
	@mkdir -p $(RESULTS)
	$(GO_TEST) ./$* | tee $(RESULTS)/$*.txt

.PHONY: compare
compare:
	go tool benchstat -ignore pkg $(foreach f,$(FRAMEWORKS),$(if $(wildcard $(RESULTS)/$(f).txt),$(f)=$(RESULTS)/$(f).txt))

.PHONY: table
table:
	@go run ./cmd/table -skip '^Grain' $(foreach f,$(FRAMEWORKS),$(if $(wildcard $(RESULTS)/$(f).txt),$(f)=$(RESULTS)/$(f).txt))

.PHONY: table-grains
table-grains:
	@go run ./cmd/table -match '^Grain' $(foreach f,$(GRAIN_FRAMEWORKS),$(if $(wildcard $(RESULTS)/$(f).txt),$(f)=$(RESULTS)/$(f).txt))

.PHONY: smoke
smoke:
	go test -run '^$$' -bench . -benchtime 1x ./...

.PHONY: vendor
vendor:
	go mod tidy && go mod vendor

.PHONY: clean
clean:
	rm -rf $(RESULTS)
