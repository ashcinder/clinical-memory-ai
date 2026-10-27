package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"linji/internal/server"
)

func main() {
	webOnly := flag.Bool("web", false, "独立启动 Web 静态站点，并将 /api 转发到后端")
	flag.Parse()
	c, err := server.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	var handler http.Handler
	addr := c.Addr
	var closeDB func()
	if *webOnly {
		backend := c.Addr
		host, port, splitErr := splitAddress(backend)
		if splitErr != nil {
			log.Fatal(splitErr)
		}
		if host == "0.0.0.0" || host == "::" || host == "" {
			host = "127.0.0.1"
		}
		target, _ := url.Parse("http://" + joinAddress(host, port))
		proxy := httputil.NewSingleHostReverseProxy(target)
		originalDirector := proxy.Director
		proxy.Director = func(r *http.Request) {
			originalDirector(r)
			r.Host = target.Host
			if r.Header.Get("Origin") != "" {
				r.Header.Set("Origin", target.String())
			}
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(502)
			_, _ = w.Write([]byte(`{"detail":"无法连接 Go 后端，请先启动后端服务"}`))
		}
		mux := http.NewServeMux()
		mux.Handle("/api/", proxy)
		mux.Handle("/", http.FileServer(http.Dir(c.WebDir)))
		handler = server.Headers(mux)
		addr = c.WebAddr
	} else {
		app, err := server.New(c)
		if err != nil {
			log.Fatal(err)
		}
		handler = app.Handler()
		closeDB = func() { _ = app.DB.Close() }
	}
	if closeDB != nil {
		defer closeDB()
	}
	httpServer := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 180 * time.Second, WriteTimeout: 190 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()
	log.Printf("临记 Go 服务：http://%s", addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
