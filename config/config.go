package config

import (
	"github.com/kelseyhightower/envconfig"
	"time"
)

type Config struct {
	Api struct {
		Port     uint16 `envconfig:"API_PORT" default:"50051" required:"true"`
		Telegram struct {
			Ids      []int32  `envconfig:"API_TELEGRAM_IDS" required:"true"`
			Hashes   []string `envconfig:"API_TELEGRAM_HASHES" required:"true"`
			Phones   []string `envconfig:"API_TELEGRAM_PHONES" required:"true"`
			Password string   `envconfig:"API_TELEGRAM_PASS" default:""`
		}
		Writer struct {
			Uri string `envconfig:"API_WRITER_URI" default:"resolver:50051" required:"true"`
		}
	}
	Db  DbConfig
	Log struct {
		Level int `envconfig:"LOG_LEVEL" default:"-4" required:"true"`
	}
	Replica ReplicaConfig
}

type DbConfig struct {
	Uri      string `envconfig:"DB_URI" default:"mongodb://localhost:27017/?retryWrites=true&w=majority" required:"true"`
	Name     string `envconfig:"DB_NAME" default:"sources" required:"true"`
	UserName string `envconfig:"DB_USERNAME" default:""`
	Password string `envconfig:"DB_PASSWORD" default:""`
	Table    struct {
		Name      string        `envconfig:"DB_TABLE_NAME" default:"tgchans" required:"true"`
		Retention time.Duration `envconfig:"DB_TABLE_RETENTION" default:"2160h" required:"true"`
		Shard     bool          `envconfig:"DB_TABLE_SHARD" default:"true"`
	}
	Tls struct {
		Enabled  bool `envconfig:"DB_TLS_ENABLED" default:"false" required:"true"`
		Insecure bool `envconfig:"DB_TLS_INSECURE" default:"false" required:"true"`
	}
}

type ReplicaConfig struct {
	Name string `envconfig:"REPLICA_NAME" required:"true"`
}

func NewConfigFromEnv() (cfg Config, err error) {
	err = envconfig.Process("", &cfg)
	return
}
