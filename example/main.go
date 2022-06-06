package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/olivere/generichttp"
)

func main() {
	var (
		port = flag.String("port", os.Getenv("PORT"), "HTTP port to bind to")
	)
	flag.Parse()

	// globalCtx can be used to ask processes to cancel once main cancels
	globalCtx, globalCancel := context.WithCancel(context.Background())
	defer globalCancel()

	// Bind to ":port"
	lis, err := net.Listen("tcp", net.JoinHostPort("", *port))
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Listening on %s", lis.Addr().String())

	// Create and start HTTP server
	srv := &http.Server{
		Handler: newApp(),

		// See e.g. https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		MaxHeaderBytes:    8 * 1024, // 8 KiB
	}
	go func() {
		if err := srv.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	// Triggers ctx.Done() if one of the signals are raised
	ctx, stop := signal.NotifyContext(globalCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	stop()

	log.Print("Shutting down")

	// Cancel dependent processes
	globalCancel()

	// Shutdown HTTP server with a 5 second leeway
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}

// App that handles our requests.
type App struct {
	mux *http.ServeMux
}

// newApp initializes an App.
func newApp() *App {
	app := &App{
		mux: http.NewServeMux(),
	}

	app.mux.Handle("/", app.rootHandler())
	app.mux.Handle("/add", app.addHandler())

	return app
}

// Route all requests to the mux.
func (app *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app.mux.ServeHTTP(w, r)
}

// rootHandler handles the "/" endpoint.
func (app *App) rootHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Time: %s\n", time.Now())
	})
}

type addRequest struct {
	A int `json:"a"`
	B int `json:"b"`
}

type addResponse struct {
	Result int `json:"result"`
}

// add implements an add handler.
//
// Request:
//   POST /add
//   Content-Type: application/json
//   {
//     "a": 1,
//     "b": 2
//   }
//
// Response:
//   Content-Type: application/json
//
//   {"result": 3}
func (app *App) addHandler() http.Handler {
	return generichttp.JSON(func(w http.ResponseWriter, req generichttp.Request[addRequest]) (*generichttp.Response[addResponse], error) {
		if req.Data == nil {
			return nil, generichttp.BadRequestError{Message: "Missing request data"}
		}
		resp := generichttp.NewResponse(&addResponse{
			Result: req.Data.A + req.Data.B,
		})
		return resp, nil
	})
}
