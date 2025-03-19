module github.com/roadrunner-server/grpc/v5

go 1.24

toolchain go1.24.0

require (
	github.com/emicklei/proto v1.14.0
	github.com/prometheus/client_golang v1.21.1
	github.com/roadrunner-server/api/v4 v4.18.1
	github.com/roadrunner-server/endure/v2 v2.6.1
	github.com/roadrunner-server/errors v1.4.1
	github.com/roadrunner-server/goridge/v3 v3.8.3
	github.com/roadrunner-server/pool v1.1.3
	github.com/roadrunner-server/tcplisten v1.5.2
	github.com/stretchr/testify v1.10.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.60.0
	go.opentelemetry.io/contrib/propagators/jaeger v1.35.0
	go.opentelemetry.io/otel v1.35.0
	go.opentelemetry.io/otel/sdk v1.35.0
	go.uber.org/zap v1.27.0
	golang.org/x/net v0.37.0
	google.golang.org/genproto v0.0.0-20250313205543-e70fdf4c4cb4
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250313205543-e70fdf4c4cb4
	google.golang.org/grpc v1.71.0
	google.golang.org/protobuf v1.36.5
)

require (
	cloud.google.com/go v0.118.3 // indirect
	cloud.google.com/go/accessapproval v1.8.3 // indirect
	cloud.google.com/go/accesscontextmanager v1.9.3 // indirect
	cloud.google.com/go/aiplatform v1.74.0 // indirect
	cloud.google.com/go/analytics v0.26.0 // indirect
	cloud.google.com/go/apigateway v1.7.3 // indirect
	cloud.google.com/go/apigeeconnect v1.7.3 // indirect
	cloud.google.com/go/apigeeregistry v0.9.3 // indirect
	cloud.google.com/go/appengine v1.9.3 // indirect
	cloud.google.com/go/area120 v0.9.3 // indirect
	cloud.google.com/go/artifactregistry v1.16.1 // indirect
	cloud.google.com/go/asset v1.20.4 // indirect
	cloud.google.com/go/assuredworkloads v1.12.3 // indirect
	cloud.google.com/go/automl v1.14.4 // indirect
	cloud.google.com/go/baremetalsolution v1.3.3 // indirect
	cloud.google.com/go/batch v1.12.0 // indirect
	cloud.google.com/go/beyondcorp v1.1.3 // indirect
	cloud.google.com/go/bigquery v1.66.2 // indirect
	cloud.google.com/go/bigtable v1.35.0 // indirect
	cloud.google.com/go/billing v1.20.1 // indirect
	cloud.google.com/go/binaryauthorization v1.9.3 // indirect
	cloud.google.com/go/certificatemanager v1.9.3 // indirect
	cloud.google.com/go/channel v1.19.2 // indirect
	cloud.google.com/go/cloudbuild v1.22.0 // indirect
	cloud.google.com/go/clouddms v1.8.4 // indirect
	cloud.google.com/go/cloudtasks v1.13.3 // indirect
	cloud.google.com/go/compute v1.34.0 // indirect
	cloud.google.com/go/contactcenterinsights v1.17.1 // indirect
	cloud.google.com/go/container v1.42.2 // indirect
	cloud.google.com/go/containeranalysis v0.13.3 // indirect
	cloud.google.com/go/datacatalog v1.24.3 // indirect
	cloud.google.com/go/dataflow v0.10.3 // indirect
	cloud.google.com/go/dataform v0.10.3 // indirect
	cloud.google.com/go/datafusion v1.8.3 // indirect
	cloud.google.com/go/datalabeling v0.9.3 // indirect
	cloud.google.com/go/dataplex v1.22.0 // indirect
	cloud.google.com/go/dataproc/v2 v2.11.0 // indirect
	cloud.google.com/go/dataqna v0.9.3 // indirect
	cloud.google.com/go/datastore v1.20.0 // indirect
	cloud.google.com/go/datastream v1.13.0 // indirect
	cloud.google.com/go/deploy v1.26.2 // indirect
	cloud.google.com/go/dialogflow v1.66.0 // indirect
	cloud.google.com/go/dlp v1.21.0 // indirect
	cloud.google.com/go/documentai v1.35.2 // indirect
	cloud.google.com/go/domains v0.10.3 // indirect
	cloud.google.com/go/edgecontainer v1.4.1 // indirect
	cloud.google.com/go/errorreporting v0.3.2 // indirect
	cloud.google.com/go/essentialcontacts v1.7.3 // indirect
	cloud.google.com/go/eventarc v1.15.1 // indirect
	cloud.google.com/go/filestore v1.9.3 // indirect
	cloud.google.com/go/firestore v1.18.0 // indirect
	cloud.google.com/go/functions v1.19.3 // indirect
	cloud.google.com/go/gkebackup v1.6.3 // indirect
	cloud.google.com/go/gkeconnect v0.12.1 // indirect
	cloud.google.com/go/gkehub v0.15.3 // indirect
	cloud.google.com/go/gkemulticloud v1.5.1 // indirect
	cloud.google.com/go/gsuiteaddons v1.7.4 // indirect
	cloud.google.com/go/iam v1.4.0 // indirect
	cloud.google.com/go/iap v1.10.3 // indirect
	cloud.google.com/go/ids v1.5.3 // indirect
	cloud.google.com/go/iot v1.8.3 // indirect
	cloud.google.com/go/kms v1.21.0 // indirect
	cloud.google.com/go/language v1.14.3 // indirect
	cloud.google.com/go/lifesciences v0.10.3 // indirect
	cloud.google.com/go/logging v1.13.0 // indirect
	cloud.google.com/go/longrunning v0.6.4 // indirect
	cloud.google.com/go/managedidentities v1.7.3 // indirect
	cloud.google.com/go/maps v1.19.0 // indirect
	cloud.google.com/go/mediatranslation v0.9.3 // indirect
	cloud.google.com/go/memcache v1.11.3 // indirect
	cloud.google.com/go/metastore v1.14.3 // indirect
	cloud.google.com/go/monitoring v1.24.0 // indirect
	cloud.google.com/go/networkconnectivity v1.16.1 // indirect
	cloud.google.com/go/networkmanagement v1.18.0 // indirect
	cloud.google.com/go/networksecurity v0.10.3 // indirect
	cloud.google.com/go/notebooks v1.12.3 // indirect
	cloud.google.com/go/optimization v1.7.3 // indirect
	cloud.google.com/go/orchestration v1.11.4 // indirect
	cloud.google.com/go/orgpolicy v1.14.2 // indirect
	cloud.google.com/go/osconfig v1.14.3 // indirect
	cloud.google.com/go/oslogin v1.14.3 // indirect
	cloud.google.com/go/phishingprotection v0.9.3 // indirect
	cloud.google.com/go/policytroubleshooter v1.11.3 // indirect
	cloud.google.com/go/privatecatalog v0.10.4 // indirect
	cloud.google.com/go/pubsub v1.47.0 // indirect
	cloud.google.com/go/pubsublite v1.8.2 // indirect
	cloud.google.com/go/recaptchaenterprise/v2 v2.19.4 // indirect
	cloud.google.com/go/recommendationengine v0.9.3 // indirect
	cloud.google.com/go/recommender v1.13.3 // indirect
	cloud.google.com/go/redis v1.18.0 // indirect
	cloud.google.com/go/resourcemanager v1.10.3 // indirect
	cloud.google.com/go/resourcesettings v1.8.3 // indirect
	cloud.google.com/go/retail v1.19.2 // indirect
	cloud.google.com/go/run v1.9.0 // indirect
	cloud.google.com/go/scheduler v1.11.4 // indirect
	cloud.google.com/go/secretmanager v1.14.5 // indirect
	cloud.google.com/go/security v1.18.3 // indirect
	cloud.google.com/go/securitycenter v1.36.0 // indirect
	cloud.google.com/go/servicedirectory v1.12.3 // indirect
	cloud.google.com/go/shell v1.8.3 // indirect
	cloud.google.com/go/spanner v1.76.1 // indirect
	cloud.google.com/go/speech v1.26.0 // indirect
	cloud.google.com/go/storagetransfer v1.12.1 // indirect
	cloud.google.com/go/talent v1.8.0 // indirect
	cloud.google.com/go/texttospeech v1.11.0 // indirect
	cloud.google.com/go/tpu v1.8.0 // indirect
	cloud.google.com/go/trace v1.11.3 // indirect
	cloud.google.com/go/translate v1.12.3 // indirect
	cloud.google.com/go/video v1.23.3 // indirect
	cloud.google.com/go/videointelligence v1.12.3 // indirect
	cloud.google.com/go/vision/v2 v2.9.3 // indirect
	cloud.google.com/go/vmmigration v1.8.3 // indirect
	cloud.google.com/go/vmwareengine v1.3.3 // indirect
	cloud.google.com/go/vpcaccess v1.8.3 // indirect
	cloud.google.com/go/webrisk v1.10.3 // indirect
	cloud.google.com/go/websecurityscanner v1.7.3 // indirect
	cloud.google.com/go/workflows v1.13.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.63.0 // indirect
	github.com/prometheus/procfs v0.16.0 // indirect
	github.com/roadrunner-server/events v1.0.1 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250313205543-e70fdf4c4cb4 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
