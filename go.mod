module github.com/openshift/managed-upgrade-operator

go 1.19

require (
	github.com/blang/semver/v4 v4.0.0
	github.com/go-logr/logr v1.2.4
	github.com/go-openapi/runtime v0.25.0
	github.com/go-openapi/strfmt v0.21.3
	github.com/go-resty/resty/v2 v2.7.0
	github.com/golang/mock v1.5.0
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/jarcoal/httpmock v1.3.0
	github.com/jpillora/backoff v1.0.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/ginkgo/v2 v2.9.5
	github.com/onsi/gomega v1.27.7
	github.com/openshift/api v0.0.0-20230330150608-05635858d40f
	github.com/openshift/client-go v0.0.0-20220831193253-4950ae70c8ea
	github.com/openshift/cluster-version-operator v1.0.1-0.20230322131514-c659e6ccaca7
	github.com/openshift/library-go v0.0.0-20230112164258-24668b1349e6
	github.com/openshift/machine-config-operator v0.0.1-0.20230330142923-2832f049b3f4
	github.com/openshift/operator-custom-metrics v0.5.1
	github.com/openshift/osde2e-common v0.0.0-20230627123718-79ec4c181d85
	github.com/operator-framework/operator-lib v0.9.0
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.64.0
	github.com/prometheus-operator/prometheus-operator/pkg/client v0.64.0
	github.com/prometheus/alertmanager v0.25.0
	github.com/prometheus/client_golang v1.15.1
	github.com/sykesm/zap-logfmt v0.0.4
	go.uber.org/zap v1.24.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.27.2
	k8s.io/apimachinery v0.27.2
	k8s.io/client-go v0.27.2
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f
	sigs.k8s.io/controller-runtime v0.15.0
	sigs.k8s.io/controller-tools v0.11.3
)

require (
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.10.1 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.2.4 // indirect
	github.com/go-openapi/analysis v0.21.4 // indirect
	github.com/go-openapi/errors v0.20.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/loads v0.21.2 // indirect
	github.com/go-openapi/spec v0.20.7 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-openapi/validate v0.22.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gobuffalo/flect v0.3.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.43.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/spf13/cobra v1.6.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.mongodb.org/mongo-driver v1.11.1 // indirect
	go.opentelemetry.io/otel v1.11.1 // indirect
	go.opentelemetry.io/otel/trace v1.11.1 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/net v0.10.0 // indirect
	golang.org/x/oauth2 v0.7.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/term v0.8.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.9.1 // indirect
	gomodules.xyz/jsonpatch/v2 v2.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.27.2 // indirect
	k8s.io/component-base v0.27.2 // indirect
	k8s.io/klog/v2 v2.90.1 // indirect
	k8s.io/utils v0.0.0-20230313181309-38a27ef9d749 // indirect
	sigs.k8s.io/e2e-framework v0.2.0 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)
