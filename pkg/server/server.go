package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"
)

type Server struct {
	server          *http.Server
	shutdownChannel chan struct{}
	running         bool
}

func NewServer(listenAddress string, listenPort int, globalShutdownChan chan struct{}) *Server {
	engine := createEngine()

	s := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", listenAddress, listenPort),
		Handler: engine,
	}

	return &Server{
		server:          s,
		shutdownChannel: globalShutdownChan,
		running:         false,
	}
}

func (s *Server) Run() error {
	var serverWg sync.WaitGroup
	serverWg.Add(1)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Panicf("listen: %s", err)
		}
	}()

	s.running = true
	log.Info("successfully started the gin server...")
	log.Info("Server is waiting on global shutdown channel broadcast...")

	go func() {
		defer serverWg.Done()
		for {
			select {
			case <-s.shutdownChannel:
				log.Warn("Server recieved shutdown signal. Shutting down...")
				err := s.shutdownServer()
				if err != nil {
					log.Panicf("Failed to cleanly shutown server %v", err)
				}
				return
			}
		}
	}()
	serverWg.Wait()

	return nil
}

func (s *Server) IsRunning() bool {
	return s.running
}

func (s *Server) Shutdown() error {
	if !s.running {
		return fmt.Errorf("Gin server is already stopped")
	}

	s.running = false
	return s.shutdownServer()
}

func (s *Server) shutdownServer() error {
	s.running = false

	//TODO: ensure gin has successfully drained all connections
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown: %s", err)
		return err
	}

	return nil
}

func createEngine() *gin.Engine {
	engine := gin.New()

	engine.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/healthcheck"},
	}))

	engine.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "nlb-attacher")
	})

	engine.GET("/healthcheck", func(c *gin.Context) {
		c.String(http.StatusOK, "healthy")
	})

	return engine
}
