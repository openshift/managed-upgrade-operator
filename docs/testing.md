# Testing

Tests are playing a primary role and we take them seriously.
It is expected from PRs to add, modify or delete tests on case by case scenario.
To contribute you need to be familiar with:

* [Ginkgo](https://github.com/onsi/ginkgo) - BDD Testing Framework for Go
* [Gomega](https://onsi.github.io/gomega/) - Matcher/assertion library

## Ginkgo

```zsh
go get -u github.com/onsi/ginkgo/ginkgo  # installs the ginkgo CLI
go get -u github.com/onsi/gomega/...     # fetches the Matcher/Assertion library
```

## How to run the tests

* You can run the tests using `make test` or `go test ./...`

