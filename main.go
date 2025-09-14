package main

import (
	"log"
	//"os"
	"time"

	"github.com/hadarco13/mini-seller/cmd/server"
	"github.com/hadarco13/mini-seller/internal/models"
	"github.com/pkg/errors"
)

const (
	configPath = "./config/environments/"
	envKeyEnv  = "env"
)

func main() {

	/*	env := os.Getenv(envKeyEnv)
		if env == "" {
			log.Fatalf("Error: env is not defined")
		}

		log.Printf("Mini Seller env: %s", env)
		conf, err := models.LoadConfig(configPath, env)
		if err != nil {
			log.Fatalf("Error while opening conf: %v", err)
		}

		err = models.PrepareEnv()
		if err != nil {
			log.Fatalf("Error while loading env file: %v", errors.Cause(err).Error())
			return
		}*/

	//define configuration of the server hardcoded till we will have env configuration
	conf := models.ServerConfig{
		Port:                "8080",
		Host:                "localhost",
		LogLevel:            "info",
		Env:                 "development",
		InitialisationStart: time.Now(),
	}
	server, err := server.NewServer(&conf)
	if err != nil {
		log.Fatalf("Error while creating server: %v", errors.Cause(err).Error())
	}

	if err = server.Start(); err != nil {
		log.Fatalf("Error while starting server: %v", err)
	}

}
