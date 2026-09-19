// snapraid-exporter exposes SnapRAID health as Prometheus metrics by reading
// the REST API of snapraid-daemon (https://github.com/amadvance/snapraid-daemon).
// It needs no privileges: the daemon serves cached state and a scrape never
// probes or spins up disks.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	listen := flag.String("web.listen-address", ":9634", "address to listen on")
	url := flag.String("daemon.url", "http://127.0.0.1:7627", "base URL of snapraid-daemon")
	timeout := flag.Duration("daemon.timeout", 10*time.Second, "timeout for daemon requests")
	flag.Parse()

	c := &client{
		base: *url,
		http: &http.Client{Timeout: *timeout},
		user: os.Getenv("SNAPRAIDD_USER"),
		pass: os.Getenv("SNAPRAIDD_PASSWORD"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(rw http.ResponseWriter, r *http.Request) {
		out, err := collect(r.Context(), c)
		if err != nil {
			log.Printf("collect: %v", err)
		}
		rw.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = rw.Write([]byte(out))
	})
	mux.HandleFunc("/", func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte(`<a href="/metrics">metrics</a>`))
	})
	log.Printf("listening on %s, daemon at %s", *listen, *url)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
