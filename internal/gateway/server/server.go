package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type Server struct {
	httpServer *http.Server
}

func New(port string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:    port,
			Handler: h2c.NewHandler(handler, &http2.Server{}),
		},
	}
}

func (s *Server) Start() error {
	log.Printf("Starting API server on %s", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
