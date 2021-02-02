include boilerplate/generated-includes.mk

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update

# TODO: Remove once prow is standardized
.PHONY: verify
verify: lint

.PHONY: run
run: 
	operator-sdk run --local --watch-namespace ""

.PHONY: tools
tools:
	cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %

.PHONY: manifests
manifests:
	./hack/generate-local-operator-bundle.sh
