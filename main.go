package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	//"time"
	log "github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"

	"github.com/birdrides/nlb-attacher/pkg/config"
	"github.com/birdrides/nlb-attacher/pkg/controller"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.DebugLevel)
}

func main() {
	fmt.Println("NLB_ATTACHER")
	var ns string
	flag.StringVar(&ns, "namespace", "", "namespace")

	r := gin.New()

	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/healthcheck"},
	}))

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "nlb-attacher")
	})

	r.GET("/healthcheck", func(c *gin.Context) {
		c.String(http.StatusOK, "healthy")
	})

	go func() {
		r.Run()
	}()

	//todo make this config come from CLI or possibly vault
	config := config.CreateConfig(false)

	c := new(controller.Controller)
	c.Init(config)
}
