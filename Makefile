include boilerplate/generated-includes.mk

.PHONY: boilerplate-update
boilerplate-update: ## Make boilerplate update itself
	@boilerplate/update

.PHONY: run
run: ## Wrapper around operator sdk run. Requires OPERATOR_NAMESPACE to be set. See run-standard for defaults. 
	operator-sdk run --local --watch-namespace ""

.PHONY: run-routes
run-routes: ## Same as `run`, however will use ROUTE objects to contact prometheus and alertmanager. Use of routes is non-standard but convenient for local development.
	ROUTES=true operator-sdk run --local --watch-namespace ""

.PHONY: run-standard
run-standard: ## Run locally with openshift-managed-upgrade-operator as OPERATOR_NAMESPACE.
	OPERATOR_NAMESPACE=openshift-managed-upgrade-operator operator-sdk run --local --watch-namespace ""

.PHONY: run-standard-routes
run-standard-routes: ## Run locally with openshift-managed-upgrade-operator as OPERATOR_NAMESPACE and use of non-standard routes.
	OPERATOR_NAMESPACE=openshift-managed-upgrade-operator ROUTES=true operator-sdk run --local --watch-namespace ""

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
