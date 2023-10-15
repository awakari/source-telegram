package config

import (
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slog"
	"os"
	"testing"
)

func TestConfig(t *testing.T) {
	os.Setenv("API_WRITER_BACKOFF", "23h")
	os.Setenv("API_WRITER_URI", "writer:56789")
	os.Setenv("LOG_LEVEL", "4")
	os.Setenv("API_TELEGRAM_ID", "123456789")
	os.Setenv("API_TELEGRAM_HASH", "deadcodecafebeef")
	os.Setenv("API_TELEGRAM_FEED_CHAT_IDS", "-1001754252633,-1001260622817,-1001801930101")
	cfg, err := NewConfigFromEnv()
	assert.Nil(t, err)
	assert.Equal(t, "writer:56789", cfg.Api.Writer.Uri)
	assert.Equal(t, slog.LevelWarn, slog.Level(cfg.Log.Level))
	assert.Equal(t, int32(123456789), cfg.Api.Telegram.Id)
	assert.Equal(t, "deadcodecafebeef", cfg.Api.Telegram.Hash)
	assert.Equal(t, []int64{
		-1001754252633,
		-1001260622817,
		-1001801930101,
	}, cfg.Api.Telegram.Feed.ChatIds)
}