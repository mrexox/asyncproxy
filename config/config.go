package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server struct {
		Bind            string        `mapstructure:"bind"`
		ResponseStatus  int           `mapstructure:"response_status"`
		ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
		Concurrency     int           `mapstructure:"concurrency"`
	} `mapstructure:"server"`

	Metrics struct {
		Bind string `mapstructure:"bind"`
		Path string `mapstructure:"path"`
	} `mapstructure:"metrics"`

	Proxy struct {
		RemoteUrl      string        `mapstructure:"remote_url"`
		RequestTimeout time.Duration `mapstructure:"request_timeout"`
		NumClients     int           `mapstructure:"num_clients"`
	} `mapstructure:"proxy"`

	Queue struct {
		Enabled         bool `mapstructure:"enabled"`
		Workers         int  `mapstructure:"workers"`
		HandlePerSecond int  `mapstructure:"handle_per_second"`
		MaxRetries      int  `mapstructure:"max_retries"`
	} `mapstructure:"queue"`

	Db struct {
		ConnectionString string `mapstructure:"connection_string"`
		MaxConnections   int    `mapstructure:"max_connections"`
	} `mapstructure:"db"`
}

func LoadConfig(path string) (*Config, error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
