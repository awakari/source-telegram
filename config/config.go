package config

import (
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Api struct {
		Telegram struct {
			Id    int32  `envconfig:"API_TELEGRAM_ID" required:"true"`
			Hash  string `envconfig:"API_TELEGRAM_HASH" required:"true"`
			Phone string `envconfig:"API_TELEGRAM_PHONE" required:"true"`
			Feed  FeedConfig
		}
		Writer struct {
			Uri string `envconfig:"API_WRITER_URI" default:"resolver:50051" required:"true"`
		}
	}
	Log struct {
		Level int `envconfig:"LOG_LEVEL" default:"-4" required:"true"`
	}
}

type FeedConfig struct {
	ChatIds []int64 `envconfig:"API_TELEGRAM_FEED_CHAT_IDS" required:"true"`
}

func NewConfigFromEnv() (cfg Config, err error) {
	err = envconfig.Process("", &cfg)
	return
}
