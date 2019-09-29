module github.com/yukirii/cluster-api-provider-vultr

go 1.12

require (
	github.com/go-logr/logr v0.1.0
	github.com/k0kubun/pp v3.0.1+incompatible
	github.com/labstack/gommon v0.3.0
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/pkg/errors v0.8.1
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	sigs.k8s.io/cluster-api v0.2.3
	sigs.k8s.io/controller-runtime v0.2.2
)
