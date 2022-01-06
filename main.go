package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"runtime"
	"sync"

	log "github.com/schollz/logger"
)

var flagDebug bool
var flagPort int

func init() {
	flag.IntVar(&flagPort, "port", 9222, "port for server")
	flag.BoolVar(&flagDebug, "debug", false, "debug mode")
}
func main() {
	// use all of the processors
	runtime.GOMAXPROCS(runtime.NumCPU())
	if flagDebug {
		log.SetLevel("debug")
	} else {
		log.SetLevel("info")
	}
	if err := serve(); err != nil {
		panic(err)
	}
}

type stream struct {
	b    []byte
	done bool
}

// Serve will start the server
func serve() (err error) {
	channels := make(map[string]map[float64]chan stream)
	mutex := &sync.Mutex{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		log.Debugf("opened %s %s", r.Method, r.URL.Path)
		defer func() {
			log.Debugf("finished %s\n", r.URL.Path)
		}()

		mutex.Lock()
		if _, ok := channels[r.URL.Path]; !ok {
			channels[r.URL.Path] = make(map[float64]chan stream)
		}
		mutex.Unlock()

		if r.Method == "GET" {
			id := rand.Float64()
			mutex.Lock()
			channels[r.URL.Path][id] = make(chan stream, 30)
			channel := channels[r.URL.Path][id]
			log.Debugf("added listener %f", id)
			mutex.Unlock()

			flusher, ok := w.(http.Flusher)
			if !ok {
				panic("expected http.ResponseWriter to be an http.Flusher")
			}
			w.Header().Set("Connection", "Keep-Alive")
			w.Header().Set("Transfer-Encoding", "chunked")

			canceled := false
			for {
				select {
				case s := <-channel:
					if s.done {
						canceled = true
					} else {
						w.Write(s.b)
						flusher.Flush()
					}
				case <-r.Context().Done():
					log.Debug("consumer canceled")
					canceled = true
				}
				if canceled {
					break
				}
			}

			mutex.Lock()
			close(channels[r.URL.Path][id])
			delete(channels[r.URL.Path], id)
			log.Debugf("removed listener %f", id)
			mutex.Unlock()
		} else if r.Method == "POST" {
			buffer := make([]byte, 2048)
			for {
				n, err := r.Body.Read(buffer)
				if err != nil {
					log.Debugf("err: %v", err)
					break
				}
				mutex.Lock()
				for _, c := range channels[r.URL.Path] {
					var b2 = make([]byte, n)
					copy(b2, buffer[:n])
					c <- stream{b: b2}
				}
				mutex.Unlock()
			}
			mutex.Lock()
			for _, c := range channels[r.URL.Path] {
				c <- stream{done: true}
			}
			mutex.Unlock()
		}
	}

	log.Infof("running on port %d", flagPort)
	err = http.ListenAndServe(fmt.Sprintf(":%d", flagPort), http.HandlerFunc(handler))
	if err != nil {
		log.Error(err)
	}
	return
}
