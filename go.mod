module github.com/kadaan/promutil

go 1.14

require (
	github.com/PaesslerAG/gval v1.0.1
	github.com/ghodss/yaml v1.0.0
	github.com/go-kit/kit v0.10.0
	github.com/json-iterator/go v1.1.9
	github.com/pkg/errors v0.9.1
	github.com/plar/go-adaptive-radix-tree v1.0.1
	github.com/prometheus/client_golang v1.5.1
	github.com/prometheus/common v0.9.1
	github.com/prometheus/prometheus v2.17.1+incompatible
	github.com/prometheus/tsdb v0.7.1
	github.com/spf13/cobra v0.0.7
	github.com/spf13/viper v1.6.3
	k8s.io/klog/v2 v2.0.0
)

replace (
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v1.8.2-0.20200326161412-ae041f97cfc6
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90
)
