package models

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hadarco13/mini-seller/internal/common/stringbuilder"
	"github.com/hadarco13/mini-seller/internal/consts"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port                string
	Host                string
	LogLevel            string
	Env                 string
	InitialisationStart time.Time `json:"-"`
}

func LoadConfig(path, env string) (*ServerConfig, error) {
	v := viper.New()

	v.AddConfigPath(path)
	v.SetConfigName(env)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("fatal error config file: %w", err)
	}

	c := new(ServerConfig)
	if err := v.Unmarshal(c); err != nil {
		return nil, fmt.Errorf("error unmarshaling config file: %w", err)
	}

	c.InitialisationStart = time.Now()

	return c, nil

}

func PrepareEnv() error {
	viper.SetConfigFile(getEnvFilePath())

	if err := viper.ReadInConfig(); err != nil {
		log.Fatal("Failed to load env file: %v", err)
		return err
	}

	return nil
}

func getEnvFilePath() string {
	envFilePath := stringbuilder.ConcatStrings(
		len(consts.EnvFilePath)+len("/")+len(os.Getenv("env"))+len(".env"),
		consts.EnvFilePath, "/", os.Getenv("env"), ".env")

	if _, err := os.Stat(envFilePath); err == nil {
		return envFilePath
	}

	return stringbuilder.ConcatStrings(
		len(consts.EnvFilePath)+len("/.env"),
		consts.EnvFilePath, "/.env")
}
