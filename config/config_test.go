package config

import (
	"github.com/stretchr/testify/assert"
	"log/slog"
	"os"
	"testing"
)

func TestConfig(t *testing.T) {
	os.Setenv("API_TELEGRAM_PHONE", "+123456789")
	os.Setenv("API_WRITER_BACKOFF", "23h")
	os.Setenv("API_WRITER_URI", "writer:56789")
	os.Setenv("LOG_LEVEL", "4")
	os.Setenv("API_TELEGRAM_ID", "123456789")
	os.Setenv("API_TELEGRAM_HASH", "deadcodecafebeef")
	os.Setenv("API_TELEGRAM_FEED_CHAT_IDS", "-1001754252633,-1001260622817,-1001801930101")
	os.Setenv("REPLICA_NAME", "replica-1")
	os.Setenv("API_TELEGRAM_IDS", "123,456,789")
	os.Setenv("API_TELEGRAM_HASHES", "deadcode,cafebeef")
	os.Setenv("API_TELEGRAM_PHONES", "1234,5678")
	cfg, err := NewConfigFromEnv()
	assert.Nil(t, err)
	assert.Equal(t, "writer:56789", cfg.Api.Writer.Uri)
	assert.Equal(t, slog.LevelWarn, slog.Level(cfg.Log.Level))
	assert.Equal(t, []int32{123, 456, 789}, cfg.Api.Telegram.Ids)
	assert.Equal(t, []string{"deadcode", "cafebeef"}, cfg.Api.Telegram.Hashes)
}
