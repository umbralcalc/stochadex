// Separate opt-in module: keeps the AWS SDK (a ~15-module dependency tree) entirely out of
// the core engine's go.mod, exactly as arrowstore does for Apache Arrow. The engine stays
// lean; consumers who want object storage opt in by importing this module.
//
// Unlike a CLI-only integration, this is an ordinary Go package: a downstream repo — which
// under the engine's repo boundary is precisely who owns data and calibration — can read a
// run from S3 or write one back without going through the binary.
module github.com/umbralcalc/stochadex/pkg/s3store

go 1.25.0

require (
	github.com/aws/aws-sdk-go-v2/config v1.32.31
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.22.35
	github.com/aws/aws-sdk-go-v2/service/s3 v1.106.0
	github.com/umbralcalc/stochadex v0.5.3
)

require (
	github.com/aws/aws-sdk-go-v2 v1.43.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.14 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.30 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.32 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.32 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.0 // indirect
	github.com/aws/smithy-go v1.27.3 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	gonum.org/v1/netlib v0.0.0-20230729102104-8b8060e7531f // indirect
	google.golang.org/protobuf v1.36.9 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

// Local development builds against the engine in the parent tree; external users get the
// pinned require above. This replace is ignored when the module is consumed as a dependency.
replace github.com/umbralcalc/stochadex => ../../
