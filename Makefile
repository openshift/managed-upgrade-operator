FIPS_ENABLED=true

include boilerplate/generated-includes.mk

OPERATOR_NAME=managed-upgrade-operator

.PHONY: boilerplate-update
boilerplate-update: ## Make boilerplate update itself
	@boilerplate/update

.PHONY: run
run:
	OPERATOR_NAMESPACE="openshift-managed-upgrade-operator" WATCH_NAMESPACE="" go run ./main.go

.PHONY: tools
tools: ## Install local go tools for MUO
	cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %

.PHONY: help
help: ## Show this help screen.
		@echo 'Usage: make <OPTIONS> ... <TARGETS>'
		@echo ''
		@echo 'Available targets are:'
		@echo ''
		@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | sed 's/##//g' | awk 'BEGIN {FS = ":"}; {printf "\033[36m%-30s\033[0m %s\n", $$2, $$3}'

.PHONY: go-mac-build
# This script is only for MacOS Developer
# Prerequisite: x86_64-unknown-linux-gnu-gcc
# brew tap SergioBenitez/osxct
# brew install x86_64-unknown-linux-gnu
go-mac-build:
	CC=x86_64-unknown-linux-gnu-gcc CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make go-build
