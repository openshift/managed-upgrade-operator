module github.com/openshift/managed-upgrade-operator

go 1.16

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/coreos/prometheus-operator v0.38.1-0.20200424145508-7e176fda06cc
	github.com/go-logr/logr v0.4.0
	github.com/go-openapi/runtime v0.19.4
	github.com/go-openapi/strfmt v0.19.5
	github.com/go-resty/resty/v2 v2.6.0
	github.com/golang/mock v1.4.4
	github.com/google/uuid v1.1.2
	github.com/hashicorp/go-multierror v1.0.0
	github.com/jarcoal/httpmock v1.0.8
	github.com/jpillora/backoff v1.0.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/openshift/api v3.9.1-0.20190424152011-77b8897ec79a+incompatible
	github.com/openshift/cluster-version-operator v3.11.1-0.20190629164025-08cac1c02538+incompatible
	github.com/openshift/library-go v0.0.0-20210825122301-7f0bf922c345
	github.com/openshift/machine-api-operator v0.2.1-0.20210917195819-eb6706653664
	github.com/openshift/machine-config-operator v0.0.1-0.20211002010814-6cf167014583
	github.com/openshift/operator-custom-metrics v0.4.2
	github.com/operator-framework/operator-sdk v0.18.2
	github.com/prometheus/alertmanager v0.20.0
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.26.0
	github.com/sykesm/zap-logfmt v0.0.4
	go.uber.org/zap v1.19.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
	sigs.k8s.io/controller-runtime v0.10.0
	sigs.k8s.io/controller-tools v0.3.0
)

replace (
	github.com/openshift/api => github.com/openshift/api v0.0.0-20210910062324-a41d3573a3ba
	k8s.io/api => k8s.io/api v0.21.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.21.1
	k8s.io/client-go => k8s.io/client-go v0.21.1
	sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20201125052318-b85a18cbf338
	sigs.k8s.io/cluster-api-provider-azure => github.com/openshift/cluster-api-provider-azure v0.0.0-20210209143830-3442c7a36c1e
)
