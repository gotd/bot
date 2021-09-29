test:
	@./go.test.sh
.PHONY: test

coverage:
	@./go.coverage.sh
.PHONY: coverage

generate:
	go generate
	go generate ./...
.PHONY: generate

check_generated: generate
	git diff --exit-code
.PHONY: check_generated

deploy:
	go build ./cmd/bot
	fab deploy -H gotd
	rm bot
