package deployable

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"

	"github.com/birdrides/nlb-attacher/pkg/config"
	"github.com/birdrides/nlb-attacher/pkg/controller"
	"github.com/birdrides/nlb-attacher/pkg/server"
)

type Deployable struct {
	server          *server.Server
	controller      *controller.Controller
	shutdownChannel chan struct{}
	running         bool
}

func NewDeployable(config *config.Config) *Deployable {
	shutdownChannel := make(chan struct{})

	s := server.NewServer(
		"0.0.0.0/0",
		8080,
		shutdownChannel,
	)

	c := controller.NewController(shutdownChannel)

	return &Deployable{
		server:          s,
		controller:      c,
		shutdownChannel: shutdownChannel,
		running:         false,
	}
}

func (d *Deployable) Run() error {
	log.Info("Will exit on SIGTERM and SIGINT.")

	gracefulStop := make(chan os.Signal)

	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()
		wg.Add(1)
		d.server.Run()
	}()

	go func() {
		defer wg.Done()
		wg.Add(1)
		d.controller.Run()
	}()

	go func() {
		defer wg.Done()
		sig := <-gracefulStop
		log.Infof("Received and broadcasting shutdown signal (%v)...", sig)
		// Close global shutdown channel
		close(d.shutdownChannel)
	}()

	wg.Wait()

	log.Info("Deployable successfully shut down.")

	return nil
}

func (d *Deployable) IsRunning() bool {
	return d.running
}
