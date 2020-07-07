# How to Contribute

## Getting started
* Fork the repository on GitHub.
* Read the [README](../README.md), [developer documentation](development.md) and [testing documentation](testing.md).
* Run a cluster upgrade driven by the operator. 
* Submit bugs or patches following the flow below.

## Contribution flow

### Issues / Bugs

Issues may be filed via [GitHub](https://github.com/openshift/managed-upgrade-operator/issues/new) or [Jira](https://issues.redhat.com/) (OSD Project).

### Contributing code changes

We recommend the following workflow when submitting change requests to this repositry:
1. Fork the repository to your own account.
2. Create a topic branch from the main branch.
3. Make commits of logical units.
4. If necessary, use `make generate` to update generated code.
5. Add or modify tests as needed. Ensure code has been [tested](testing.md) prior to PR.
6. Push your changes to a topic branch in your fork of the repository.
7. Submit a pull request to the original repository.
8. The repo [owners](../OWNERS) will respond to your issue promptly, following [the ususal Prow workflow](https://github.com/kubernetes/community/blob/master/contributors/guide/owners.md#the-code-review-process).

## Style guide

### Linting

This project makes use of [golangci-lint](https://github.com/golangci/golangci-lint) for performing code linting.

It can be run on-demand with a call to `make verify`. Pull requests that do not pass all linting checks are not approved for merge. 

The linting rules for this project are defined in a [golangci-lint configuration](../.golangci.yml).
  



