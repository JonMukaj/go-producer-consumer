package config

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// DB holds database connection settings shared by both services.
type DB struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	SSLMode  string `mapstructure:"sslmode"`
}

func (d DB) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// Producer holds all configuration for the producer service.
type Producer struct {
	PrometheusPort int    `mapstructure:"prometheus_port"`
	ProfilingPort  int    `mapstructure:"profiling_port"`
	LogLevel       string `mapstructure:"log_level"`
	LogFormat      string `mapstructure:"log_format"`

	GRPCTarget    string `mapstructure:"grpc_target"`
	RatePerSecond int    `mapstructure:"rate_per_second"`
	MaxBacklog    int    `mapstructure:"max_backlog"`

	DB DB `mapstructure:"db"`
}

// Consumer holds all configuration for the consumer service.
type Consumer struct {
	PrometheusPort int    `mapstructure:"prometheus_port"`
	ProfilingPort  int    `mapstructure:"profiling_port"`
	LogLevel       string `mapstructure:"log_level"`
	LogFormat      string `mapstructure:"log_format"`

	GRPCListen string `mapstructure:"grpc_listen"`
	RateLimit  float64 `mapstructure:"rate_limit"`
	RateBurst  int    `mapstructure:"rate_burst"`

	DB DB `mapstructure:"db"`
}

// LoadProducer loads producer config from the embedded YAML bytes, then
// overrides with any environment variables prefixed with PRODUCER_.
func LoadProducer(defaultYAML []byte) (Producer, error) {
	v := newViper("PRODUCER")
	if err := v.ReadConfig(bytes.NewReader(defaultYAML)); err != nil {
		return Producer{}, fmt.Errorf("read producer config: %w", err)
	}
	var cfg Producer
	if err := v.Unmarshal(&cfg); err != nil {
		return Producer{}, fmt.Errorf("unmarshal producer config: %w", err)
	}
	return cfg, nil
}

// LoadConsumer loads consumer config from the embedded YAML bytes, then
// overrides with any environment variables prefixed with CONSUMER_.
func LoadConsumer(defaultYAML []byte) (Consumer, error) {
	v := newViper("CONSUMER")
	if err := v.ReadConfig(bytes.NewReader(defaultYAML)); err != nil {
		return Consumer{}, fmt.Errorf("read consumer config: %w", err)
	}
	var cfg Consumer
	if err := v.Unmarshal(&cfg); err != nil {
		return Consumer{}, fmt.Errorf("unmarshal consumer config: %w", err)
	}
	return cfg, nil
}

func newViper(prefix string) *viper.Viper {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix(prefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	return v
}
