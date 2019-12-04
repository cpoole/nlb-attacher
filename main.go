package main

import (
	"math/rand"
	"os"
	"time"

	//"time"
	log "github.com/sirupsen/logrus"

	"github.com/birdrides/nlb-attacher/pkg/config"
	"github.com/birdrides/nlb-attacher/pkg/deployable"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.DebugLevel)

	rand.Seed(time.Now().Unix())
}

func main() {
	//Todo make this config come from CLI or possibly vault
	config := config.CreateConfig(false)

	app := deployable.NewDeployable(config)

	if err := app.Run(); err != nil {
		panic(err)
	}
}

//	//Todo make this config come from CLI or possibly vault
//	config := config.CreateConfig(false)
//
//	c := new(controller.Controller)
//	c.Init(config)
