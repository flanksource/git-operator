module github.com/flanksource/git-operator

go 1.14

require (
	github.com/flanksource/commons v1.4.3
	github.com/flanksource/kommons v0.1.6
	github.com/fluxcd/go-git-providers v0.0.3
	github.com/fluxcd/toolkit v0.0.1-beta.2
	github.com/go-git/go-billy v4.2.0+incompatible
	github.com/go-git/go-billy/v5 v5.0.0
	github.com/go-git/go-git/v5 v5.1.0
	github.com/go-logr/logr v0.2.1
	github.com/go-logr/zapr v0.2.0
	github.com/go-openapi/spec v0.19.8
	github.com/google/go-github/v32 v32.1.0
	github.com/jenkins-x/go-scm v1.5.141
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/nbio/st v0.0.0-20140626010706-e9e8d9816f32 // indirect
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	github.com/rjeczalik/notify v0.9.2
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/weaveworks/libgitops v0.0.3
	go.uber.org/zap v1.15.0
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9
	golang.org/x/oauth2 v0.0.0-20191202225959-858c2ad4c8b6
	golang.org/x/sys v0.0.0-20200812155832-6a926be9bd1d
	gopkg.in/src-d/go-billy.v4 v4.3.2
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.19.3
	k8s.io/apimachinery v0.19.3
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20200805222855-6aeccd4b50c6
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/kustomize/api v0.4.1
	sigs.k8s.io/kustomize/kyaml v0.1.11
	sigs.k8s.io/yaml v1.2.0

)

replace (
	github.com/docker/distribution => github.com/docker/distribution v2.7.1+incompatible
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20191230161307-f3c370f40bfb
	google.golang.org/grpc => google.golang.org/grpc v1.26.0
	k8s.io/client-go => k8s.io/client-go v0.19.1
)
