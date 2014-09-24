package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"code.google.com/p/go.net/websocket"
	flags "github.com/jessevdk/go-flags"
	"github.com/jrconlin/motrak/motrak"
	"github.com/jrconlin/motrak/util"
)

var opts struct {
	ConfigFile string `short:"c" long:"config" description:"configuration file"`
}

func main() {

	if _, err := flags.ParseArgs(&opts, os.Args); err != nil {
		log.Fatalf(err.Error())
		return
	}

	if opts.ConfigFile == "" {
		opts.ConfigFile = "config.ini"
	}

	config, err := util.ReadMzConfig(opts.ConfigFile)
	if err != nil {
		log.Fatalf(err.Error())
	}

	errChan := make(chan error)
	sigChan := make(chan os.Signal)
	Mux := http.DefaultServeMux
	host := config.Get("host", "localhost")
	port := config.Get("port", "8080")
	handlers := motrak.NewHandler(config)

	runtime.GOMAXPROCS(runtime.NumCPU())
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGUSR1)

	Mux.Handle("/ws", websocket.Handler(handlers.WSHandler))
    Mux.HandleFunc("/u", handlers.Update)
	Mux.HandleFunc("/", handlers.Index)

	log.Printf("main: starting up %s:%s\n", host, port)

	go func() {
		errChan <- http.ListenAndServe(fmt.Sprintf("%s:%s", host, port), nil)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			log.Fatalf("main: ListenAndServe: %s\n" + err.Error())
		}
	case <-sigChan:
		log.Printf("main: Shutting down...\n")
	}

}
