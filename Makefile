include boilerplate/generated-includes.mk

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update

.PHONY: run
run: 
	operator-sdk run --local --watch-namespace ""

.PHONY: tools
tools:
	cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %
