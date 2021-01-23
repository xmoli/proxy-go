// Copyright 2021, The Go Authors. All rights reserved.
// Author: crochee
// Date: 2021/1/17

package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"proxy-go/config/dynamic"
	"sync"
	"time"

	"proxy-go/config"
	"proxy-go/logger"
	"proxy-go/safe"
)

type Server struct {
	config   *config.Config
	pool     *safe.Pool
	list     map[string]*http.Server
	handler  http.Handler
	stopChan chan struct{}
	ctx      context.Context
	lock     sync.RWMutex
	wg       sync.WaitGroup
}

// New returns an initialized Server.
func New(ctx context.Context, routinesPool *safe.Pool, cf *config.Config, handler http.Handler) *Server {
	return &Server{
		config:   cf,
		pool:     routinesPool,
		list:     make(map[string]*http.Server, len(cf.Server.Medata)),
		handler:  handler,
		stopChan: make(chan struct{}, 1),
		ctx:      ctx,
	}
}

func (s *Server) Start() {
	go func() {
		<-s.ctx.Done()
		logger.FromContext(s.ctx).Info("Stopping server gracefully")
		s.Stop()
	}()
	if s.config.Server == nil {
		s.Stop()
	}
	for _, medata := range s.config.Server.Medata {
		s.wg.Add(1)
		go func(m *dynamic.Medata) {
			s.listen(m)
			s.wg.Done()
		}(medata)
	}
}

// Wait blocks until the server shutdown.
func (s *Server) Wait() {
	s.wg.Wait()
	<-s.stopChan
}

// Stop stops the server.
func (s *Server) Stop() {
	for name, srv := range s.list {
		s.wg.Add(1)
		var graceTimeOut time.Duration
		for _, medata := range s.config.Server.Medata {
			if medata.Name == name {
				graceTimeOut = medata.GraceTimeOut
			}
		}
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)
		if graceTimeOut > 0 {
			ctx, cancel = context.WithCancel(s.ctx)
		} else {
			ctx, cancel = context.WithTimeout(s.ctx, graceTimeOut)
		}

		go func(ctx context.Context, server *http.Server) {
			Shutdown(ctx, server)
			s.wg.Done()
		}(ctx, srv)
		cancel()
	}
	s.stopChan <- struct{}{}
	logger.FromContext(s.ctx).Info("server stopped")
}

// Close destroys the server.
func (s *Server) Close() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)

	go func(ctx context.Context) {
		<-ctx.Done()
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			panic("timeout while stopping proxy, killing instance ✝")
		}
	}(ctx)

	s.pool.Stop()

	close(s.stopChan)
	cancel()
}

func Shutdown(ctx context.Context, server *http.Server) {
	err := server.Shutdown(ctx)
	if err == nil {
		return
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		logger.FromContext(ctx).Debugf("server failed to shutdown within deadline because: %s", err)
		if err = server.Close(); err != nil {
			logger.Error(err.Error())
		}
		return
	}
	logger.FromContext(ctx).Error(err.Error())
	// We expect Close to fail again because Shutdown most likely failed when trying to close a listener.
	// We still call it however, to make sure that all connections get closed as well.
	_ = server.Close()
}

func (s *Server) listen(m *dynamic.Medata) {
	log := logger.FromContext(s.ctx)
	addr := fmt.Sprintf(":%d", m.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error(err.Error())
		return
	}
	srv := &http.Server{Handler: s.handler}
	switch m.Scheme {
	case "http":
		log.Infof("http server medata:%+v running...", m)
	case "https":
		if m.Tls == nil {
			log.Error("https haven't tls")
			return
		}
		certPEMBlock, err := m.Tls.Cert.Read()
		if err != nil {
			logger.Error(err.Error())
			return
		}
		keyPEMBlock, err := m.Tls.Key.Read()
		if err != nil {
			logger.Error(err.Error())
			return
		}
		certificate, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
		if err != nil {
			log.Error(err.Error())
			return
		}

		ln = tls.NewListener(ln, &tls.Config{
			Certificates:       []tls.Certificate{certificate},
			ServerName:         m.Name,
			InsecureSkipVerify: true,
		})
		log.Infof("https server medata:%+v running...", m)
	default:
	}
	go func() {
		s.lock.Lock()
		s.list[m.Name] = srv
		s.lock.Unlock()
		if err := srv.Serve(ln); err != nil {
			log.Error(err.Error())
		}
		s.lock.Lock()
		delete(s.list, m.Name)
		s.lock.Unlock()
	}()

}
