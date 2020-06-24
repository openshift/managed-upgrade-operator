# Development

This document should entail all you need to develop this operator locally. 

## Versions

### golang

`go version >= go1.13`

### operator-sdk

`operator-sdk version: "v0.17.0"`

## Dependencies

### GoMock

[`GoMock`](https://github.com/golang/mock) is used for building or re-building mock interfaces used in [testing](./testing.md).

`GO111MODULE=on go get github.com/golang/mock/mockgen@latest`

## How to run

1. Either fork your own or checkout the repo from https://github.com/openshift/managed-upgrade-operator into your working directory. For example:

```
cd ~/go/src/github.com/openshift
git clone https://github.com/openshift/managed-upgrade-operator.git
cd managed-upgrade-operator
```

2. From within the directory, run:

`make run` or `operator-sdk run --local --watch-namespace ""`

3. Trigger a reconcile loop by applying an `upgradeconfig` CR with your desired specs. 

```
oc apply -f deploy/crds/upgrade.managed.openshift.io_v1alpha1_upgradeconfig_cr.yaml
```
