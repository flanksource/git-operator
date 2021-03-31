module github.com/flanksource/git-operator

go 1.14

require (
	github.com/flanksource/commons v1.4.3
	github.com/flanksource/kommons v0.1.6
	github.com/go-git/go-billy/v5 v5.0.0
	github.com/go-git/go-git/v5 v5.1.0
	github.com/go-logr/logr v0.2.1
	github.com/go-logr/zapr v0.2.0 // indirect
	github.com/google/go-github/v32 v32.1.0
	github.com/jenkins-x/go-scm v1.5.224
	github.com/labstack/echo v3.3.10+incompatible
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/weaveworks/libgitops v0.0.3
	go.uber.org/zap v1.15.0
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/oauth2 v0.0.0-20191202225959-858c2ad4c8b6
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.19.3
	k8s.io/apimachinery v0.19.3
	k8s.io/client-go v11.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/kustomize/api v0.4.1
	sigs.k8s.io/yaml v1.2.0

)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20191230161307-f3c370f40bfb
	google.golang.org/grpc => google.golang.org/grpc v1.26.0
	k8s.io/client-go => k8s.io/client-go v0.19.1
)
