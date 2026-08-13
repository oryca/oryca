package config

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testDBURL     = "mongodb://localhost:27017"
	testRedisAddr = "127.0.0.1:6379"
)

func TestMustGetEnv_ReturnsValue(t *testing.T) {
	os.Setenv("TEST_KEY", "hello")
	defer os.Unsetenv("TEST_KEY")

	result := mustGetEnv("TEST_KEY")
	assert.Equal(t, "hello", result)
}

// ใช้ subprocess เพราะ log.Fatalf เรียก os.Exit ตรงๆ ไม่สามารถ catch ใน process เดียวกันได้
func TestMustGetEnv_FatalWhenMissing(t *testing.T) {
	os.Unsetenv("TEST_MISSING_KEY")

	cmd := exec.Command(os.Args[0], "-test.run=TestMustGetEnv_FatalWhenMissing_Helper")
	cmd.Env = append(os.Environ(), "RUN_FATAL_TEST=1")
	err := cmd.Run()

	assert.Error(t, err, "ควร exit เมื่อไม่มี env")
}

func TestMustGetEnv_FatalWhenMissing_Helper(t *testing.T) {
	if os.Getenv("RUN_FATAL_TEST") != "1" {
		t.Skip()
	}
	mustGetEnv("TEST_MISSING_KEY")
}

func setRequiredEnvs() func() {
	os.Setenv("ORYCA_API_HOST", "http://localhost")
	os.Setenv("ORYCA_API_PORT", "9001")
	os.Setenv("ORYCA_API_DB_URL", testDBURL)
	os.Setenv("ORYCA_API_DB_NAME", "oryca_test")
	os.Setenv("ORYCA_API_REDIS_ADDRESS", testRedisAddr)
	return func() {
		os.Unsetenv("ORYCA_API_HOST")
		os.Unsetenv("ORYCA_API_PORT")
		os.Unsetenv("ORYCA_API_DB_URL")
		os.Unsetenv("ORYCA_API_DB_NAME")
		os.Unsetenv("ORYCA_API_REDIS_ADDRESS")
	}
}

func TestLoad_ReturnsConfig(t *testing.T) {
	defer setRequiredEnvs()()

	cfg := Load()

	assert.Equal(t, "http://localhost", cfg.AppHost)
	assert.Equal(t, "9001", cfg.AppPort)
	assert.Equal(t, testDBURL, cfg.DBUrl)
	assert.Equal(t, "oryca_test", cfg.DBName)
	assert.Equal(t, testRedisAddr, cfg.RedisAddress)
}

func TestLoadRedisDefaults(t *testing.T) {
	defer setRequiredEnvs()()

	cfg := Load()

	assert.Equal(t, "0", cfg.RedisDB)
	assert.Equal(t, "300", cfg.RedisExpires)
	assert.Equal(t, "50", cfg.RedisPoolSize)
	assert.Equal(t, "10", cfg.RedisMinIdleConns)
	assert.Equal(t, "5", cfg.RedisDialTimeout)
	assert.Equal(t, "3", cfg.RedisReadTimeout)
	assert.Equal(t, "3", cfg.RedisWriteTimeout)
}
