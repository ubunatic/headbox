BINARY := headbox

.PHONY: ⚙️ 🤖  # ⚙️ = manual/once, 🤖 = managed

_prim := \033[36m
_rst  := \033[0m

help: 🤖  # show this help
	@grep -E '^[a-zA-Z_-]+:.*[⚙🤖].*#+' $(MAKEFILE_LIST) | \
	awk 'BEGIN {FS = ":.*#+ "}; {printf "    $(_prim)%-15s$(_rst) %s\n", $$1, $$2}'

build: ⚙️  # build the headbox binary
	go build -o $(BINARY) .

test-short: ⚙️  # unit tests only, no browser started
	go test -short ./...

test: ⚙️  # all tests; the browser tests skip unless `headbox install` ran
	go test -count=1 ./...

vet: ⚙️  # run go vet
	go vet ./...

reuse: ⚙️  # verify license compliance
	reuse lint

check: ⚙️ vet test reuse  # vet + tests + licences

release: check ⚙️  # release the project using harnez
	harnez release

install: ⚙️  # install headbox into GOBIN
	go install .

clean: ⚙️  # remove build output
	rm -rf $(BINARY) dist
