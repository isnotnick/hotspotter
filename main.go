package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

var version = "dev"

//go:embed index.html
var indexHTML []byte

func main() {
	addr := flag.String("addr", ":8080", "listen address (e.g. :8080 or 0.0.0.0:9090)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("hotspotter", version)
		os.Exit(0)
	}

	wifi := NewWifiManager()
	hotspot := NewHotspotManager(wifi)
	handlers := NewHandlers(hotspot, wifi)

	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	// Serve the embedded single-page frontend.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	fmt.Printf("HotSpotter listening on %s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
