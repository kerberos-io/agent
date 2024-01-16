module github.com/kerberos-io/agent/machinery

go 1.20

//replace github.com/kerberos-io/joy4 v1.0.63 => ../../../../github.com/kerberos-io/joy4

//replace github.com/kerberos-io/onvif v0.0.10 => ../../../../github.com/kerberos-io/onvif

require (
	github.com/InVisionApp/conjungo v1.1.0
	github.com/appleboy/gin-jwt/v2 v2.9.1
	github.com/bluenviron/gortsplib/v4 v4.7.1
	github.com/bluenviron/mediacommon v1.7.1
	github.com/cedricve/go-onvif v0.0.0-20200222191200-567e8ce298f6
	github.com/dropbox/dropbox-sdk-go-unofficial/v6 v6.0.5
	github.com/eclipse/paho.mqtt.golang v1.4.3
	github.com/elastic/go-sysinfo v1.11.2
	github.com/gin-contrib/cors v1.5.0
	github.com/gin-contrib/pprof v1.4.0
	github.com/gin-gonic/contrib v0.0.0-20221130124618-7e01895a63f2
	github.com/gin-gonic/gin v1.9.1
	github.com/gofrs/uuid v4.4.0+incompatible
	github.com/golang-jwt/jwt/v4 v4.5.0
	github.com/golang-module/carbon/v2 v2.3.4
	github.com/gorilla/websocket v1.5.1
	github.com/kellydunn/golang-geo v0.7.0
	github.com/kerberos-io/joy4 v1.0.64
	github.com/kerberos-io/onvif v0.0.14
	github.com/minio/minio-go/v6 v6.0.57
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/pion/rtp v1.8.3
	github.com/pion/webrtc/v3 v3.2.24
	github.com/sirupsen/logrus v1.9.3
	github.com/swaggo/files v1.0.1
	github.com/swaggo/gin-swagger v1.6.0
	github.com/swaggo/swag v1.16.2
	github.com/tevino/abool v1.2.0
	github.com/yapingcat/gomedia v0.0.0-20240106100545-1b855b02fa0d
	github.com/zaf/g711 v1.4.0
	go.mongodb.org/mongo-driver v1.13.1
	gopkg.in/DataDog/dd-trace-go.v1 v1.59.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/DataDog/appsec-internal-go v1.4.0 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.50.2 // indirect
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.50.2 // indirect
	github.com/DataDog/datadog-go/v5 v5.4.0 // indirect
	github.com/DataDog/go-libddwaf/v2 v2.2.3 // indirect
	github.com/DataDog/go-sqllexer v0.0.10 // indirect
	github.com/DataDog/go-tuf v1.0.2-0.5.2 // indirect
	github.com/DataDog/gostackparse v0.7.0 // indirect
	github.com/DataDog/sketches-go v1.4.4 // indirect
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/beevik/etree v1.3.0 // indirect
	github.com/bytedance/sonic v1.10.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/chenzhuoyu/base64x v0.0.0-20230717121745-296ad89f973d // indirect
	github.com/chenzhuoyu/iasm v0.9.1 // indirect
	github.com/clbanning/mxj v1.8.4 // indirect
	github.com/clbanning/mxj/v2 v2.7.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.5.1 // indirect
	github.com/elastic/go-windows v1.0.1 // indirect
	github.com/elgs/gostrgen v0.0.0-20220325073726-0c3e00d082f6 // indirect
	github.com/erikstmartin/go-testdb v0.0.0-20160219214506-8d10e4a1bae5 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-openapi/jsonpointer v0.20.2 // indirect
	github.com/go-openapi/jsonreference v0.20.4 // indirect
	github.com/go-openapi/spec v0.20.14 // indirect
	github.com/go-openapi/swag v0.22.7 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.16.0 // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/pprof v0.0.0-20231229205709-960ae82b1e42 // indirect
	github.com/google/uuid v1.5.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/joeshaw/multierror v0.0.0-20140124173710-69b34d4ec901 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/kylelemons/go-gypsy v1.0.0 // indirect
	github.com/leodido/go-urn v1.2.4 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/onsi/gomega v1.27.4 // indirect
	github.com/outcaste-io/ristretto v0.2.3 // indirect
	github.com/pelletier/go-toml/v2 v2.1.1 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pion/datachannel v1.5.5 // indirect
	github.com/pion/dtls/v2 v2.2.9 // indirect
	github.com/pion/ice/v2 v2.3.12 // indirect
	github.com/pion/interceptor v0.1.25 // indirect
	github.com/pion/logging v0.2.2 // indirect
	github.com/pion/mdns v0.0.9 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.13 // indirect
	github.com/pion/sctp v1.8.9 // indirect
	github.com/pion/sdp/v3 v3.0.6 // indirect
	github.com/pion/srtp/v2 v2.0.18 // indirect
	github.com/pion/stun v0.6.1 // indirect
	github.com/pion/transport/v2 v2.2.4 // indirect
	github.com/pion/turn/v2 v2.1.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/richardartoul/molecule v1.0.1-0.20221107223329-32cfee06a052 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.8.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	github.com/tinylib/msgp v1.1.9 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.2.12 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20201027041543-1326539a0a0a // indirect
	github.com/ziutek/mymysql v1.5.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go4.org/intern v0.0.0-20230525184215-6c62f75575cb // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20231121144256-b99613f794b6 // indirect
	golang.org/x/arch v0.7.0 // indirect
	golang.org/x/crypto v0.18.0 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/net v0.20.0 // indirect
	golang.org/x/oauth2 v0.16.0 // indirect
	golang.org/x/sync v0.6.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.16.1 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/grpc v1.60.1 // indirect
	google.golang.org/protobuf v1.32.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v1.0.1 // indirect
	inet.af/netaddr v0.0.0-20230525184311-b8eac61e914a // indirect
)
