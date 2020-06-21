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

## Writing tests

### Mocking interfaces

This project makes use of [`GoMock`](https://github.com/golang/mock) to mock service interfaces. This comes with the `mockgen` utility which can be used to generate or re-generate mock interfaces that can be used to simulate the behaviour of an external dependency.

Mocking can be performed using the `mockgen` utility, which is installed via:

`GO111MODULE=on go get github.com/golang/mock/mockgen@latest`

Once installed, an interface can be mocked by running: 

`mockgen -s=/path/to/file_containing_interface.go > /path/to/output_mock_file.go`

However, it is considered good practice to include a [go generate](https://golang.org/pkg/cmd/go/internal/generate/) directive above the interface which defines the specific `mockgen` command that will generate your mocked interface. 

Internal interfaces including `pkg/maintenance/maintenance.go` and `pkg/controller/upgradeconfig/cluster_upgrader.go` are mocked using this method. When making changes to these packages, you should re-generate the mocks to ensure they too are updated. This can be performed manually by running `go generate /path/to/file.go` or for the whole project via `make generate`.
