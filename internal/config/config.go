package config

import (
	_ "github.com/joho/godotenv/autoload"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	// Server
	ServerPort int    `envconfig:"SERVER_PORT" default:"8080"`
	LogLevel   string `envconfig:"LOG_LEVEL"   default:"info"`

	// Database
	DatabaseDSN string `envconfig:"DATABASE_DSN" required:"true"`
	DBMaxOpen   int    `envconfig:"DB_MAX_OPEN"  default:"25"`
	DBMaxIdle   int    `envconfig:"DB_MAX_IDLE"  default:"10"`

	// JWT & Encryption
	//
	// JWTSecret is optional: when left empty the signing key is loaded from the
	// system_configs table (auto-generated and AES-256-GCM encrypted on first
	// boot).  Set it explicitly only to override the DB value (e.g. key rotation).
	JWTSecret      string `envconfig:"JWT_SECRET"       required:"false"`
	JWTExpiryHours int    `envconfig:"JWT_EXPIRY_HOURS" default:"24"`
	// EncryptionKey is the AES-256 master key (base64-encoded 32 bytes).
	// It is the only secret that MUST remain in the environment – it is used to
	// encrypt/decrypt all other secrets stored in system_configs.
	EncryptionKey string `envconfig:"ENCRYPTION_KEY" required:"true"`

	// AI Worker goroutine pool
	AIWorkers int `envconfig:"AI_WORKERS" default:"2"`

	// AI Tool data sources (Prometheus + OpenSearch = full RCA capability)
	PrometheusURL string `envconfig:"PROMETHEUS_URL"` // e.g. http://prometheus:9090
	OpenSearchURL string `envconfig:"OPENSEARCH_URL"` // e.g. http://opensearch:9200

	// K8s (optional)
	K8sEnabled     bool   `envconfig:"K8S_ENABLED"      default:"false"`
	K8sClusterName string `envconfig:"K8S_CLUSTER_NAME" default:"default"`
	K8sKubeconfig  string `envconfig:"K8S_KUBECONFIG"`

	// Redis (optional, Phase 3)
	RedisEnabled  bool   `envconfig:"REDIS_ENABLED"  default:"false"`
	RedisAddr     string `envconfig:"REDIS_ADDR"     default:"localhost:6379"`
	RedisPassword string `envconfig:"REDIS_PASSWORD"`

	// 注：Kafka consumer 不接受任何 env 控制。Brokers / topic / filter /
	// mapping / SASL / TLS / rate-limit 全部按行存在 data_sources 表中，由
	// KafkaManager 通过 pg_notify('data_source_event') 热加载——表里有行就
	// 起 Reader，没行就空跑。alertmesh 仅作 Kafka 消费者，没有 producer /
	// sink 路径。
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("ALERTMESH", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
