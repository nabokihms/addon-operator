module github.com/flant/addon-operator

go 1.12

require (
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/flant/shell-operator v1.0.0-beta.5.0.20191004152159-0a045385ab30 // branch: master
	github.com/go-openapi/spec v0.19.3
	github.com/kennygrant/sanitize v1.2.4
	github.com/otiai10/copy v1.0.1
	github.com/peterbourgon/mergemap v0.0.0-20130613134717-e21c03b7a721
	github.com/prometheus/client_golang v1.0.0
	github.com/romana/rlog v0.0.0-20171115192701-f018bc92e7d7
	github.com/segmentio/go-camelcase v0.0.0-20160726192923-7085f1e3c734
	github.com/stretchr/testify v1.4.0
	golang.org/x/tools v0.0.0-20190627033414-4874f863e654 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/satori/go.uuid.v1 v1.2.0
	gopkg.in/yaml.v2 v2.2.2
	k8s.io/api v0.0.0-20190409092523-d687e77c8ae9
	k8s.io/apimachinery v0.0.0-20190409092423-760d1845f48b
	k8s.io/client-go v0.0.0-20190411052641-7a6b4715b709
	k8s.io/utils v0.0.0-20190308190857-21c4ce38f2a7
	sigs.k8s.io/yaml v1.1.0
)

replace github.com/go-openapi/validate => github.com/flant/go-openapi-validate v0.19.4-0.20190926112101-38fbca4ac77f // branch: fix_in_body
