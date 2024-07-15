
The `cr-client.go` is a lib required by mockgen tests. This lib is created from the controller-runtime client. With change in controller-runtime, we need to update the `cr-client.go` file as well. Using the command line, the following command can be used to update the `cr-client.go` file:

```bash
$ mockgen -package mocks -destination=cr-client.go sigs.k8s.io/controller-runtime/pkg/client Client,StatusWriter,Reader,Writer,SubResourceClient
```
