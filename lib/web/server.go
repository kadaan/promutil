package web

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/errors"
	"github.com/kadaan/promutil/lib/remote"
	"github.com/kadaan/promutil/lib/web/ui/static"
	"github.com/kadaan/promutil/version"
	"k8s.io/klog/v2"
	"log"
	"net/http"
	"sort"
	"time"
)

func newServer(c *config.WebConfig) (Server, error) {
	alertTesterRoute, err := NewAlertTester(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create alert tester")
	}

	svr := &server{
		config: *c,
		routes: []Route{
			alertTesterRoute,
		},
	}
	return svr, nil
}

type Server interface {
	Start() (RunningServer, error)
}

type RunningServer interface {
	Stop()
}

type NavBarLink struct {
	Path string
	Name string
}

type Route interface {
	GetOrder() int

	GetDefault() *string

	GetNavBarLinks() []NavBarLink

	Register(router gin.IRouter, templateExecutor TemplateExecutor, queryable remote.Queryable)
}

type serverState int

const (
	stopped serverState = iota
	started
)

type server struct {
	state      serverState
	config     config.WebConfig
	httpServer http.Server
	routes     []Route
}

func (s *server) createServer() error {
	queryable, err := remote.NewQueryable(s.config.Host)
	if err != nil {
		return err
	}
	options := s.newOptions()
	tmplExecutor := NewTemplateExecutor(options)
	router := s.createRouter(tmplExecutor, queryable)
	s.httpServer = http.Server{
		Addr:    s.config.ListenAddress.String(),
		Handler: router,
	}
	return nil
}

func (s *server) Start() (RunningServer, error) {
	if s.state == stopped {
		s.state = started
		if err := s.createServer(); err != nil {
			return nil, errors.Wrap(err, "cannot start server")
		}
		var err error
		go func() {
			klog.V(0).Infof("Started server on %s", s.config.ListenAddress)
			if err = s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				klog.Errorf("Failed to start server: %s", err)
			}
		}()
		return s, err
	}
	return nil, errors.New("cannot start server because it is already running")
}

func (s *server) Stop() {
	if s.state == started {
		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer func() {
			cancel()
			s.state = stopped
		}()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Panicf("Server shutdown failed:%s", err)
		}
		log.Println("Server shutdown")
	}
}

func (s *server) createRouter(tmplExecutor TemplateExecutor, queryable remote.Queryable) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.UseRawPath = true
	router.RedirectTrailingSlash = true
	router.Use(gin.Recovery())
	router.StaticFS("/static", static.Static)

	defaultRouteSet := false
	for _, route := range s.routes {
		if defaultRoute := route.GetDefault(); defaultRoute != nil && !defaultRouteSet {
			router.GET("/", func(c *gin.Context) {
				c.Redirect(http.StatusFound, *defaultRoute)
			})
			defaultRouteSet = true
		}
		route.Register(router, tmplExecutor, queryable)
	}
	return router
}

func (s *server) newOptions() *Options {
	var navBarLinkOrder []int
	navBarLinkMap := make(map[int][]NavBarLink, len(s.routes))
	for _, route := range s.routes {
		if _, ok := navBarLinkMap[route.GetOrder()]; !ok {
			navBarLinkOrder = append(navBarLinkOrder, route.GetOrder())
			navBarLinkMap[route.GetOrder()] = []NavBarLink{}
		}
		navBarLinkMap[route.GetOrder()] = append(navBarLinkMap[route.GetOrder()], route.GetNavBarLinks()...)
	}
	sort.Ints(navBarLinkOrder)
	var navBarLinks []NavBarLink
	for _, order := range navBarLinkOrder {
		navBarLinks = append(navBarLinks, navBarLinkMap[order]...)
	}
	return &Options{
		Version:     version.NewInfo(),
		NavBarLinks: navBarLinks,
	}
}
