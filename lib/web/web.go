package web

import (
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/command"
	"github.com/kadaan/promutil/lib/errors"
	"log"
	"os"
	"os/signal"
)

func NewWeb() command.Task[config.WebConfig] {
	return &web{}
}

type web struct {
}

func (t *web) Run(c *config.WebConfig) error {
	quit := make(chan os.Signal)
	svr, err := newServer(c)
	if err != nil {
		return errors.Wrap(err, "failed to create server")
	}
	runningSvr, err := svr.Start()
	if runningSvr != nil {
		defer runningSvr.Stop()
	}
	if err != nil {
		log.Println("Shutting down server...")
		return err
	}
	log.Println("Press Ctrl-C to shutdown server")
	signal.Notify(quit, os.Interrupt)
	<-quit
	runningSvr.Stop()
	return nil
}
