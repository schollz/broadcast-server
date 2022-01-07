package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/h2non/filetype"
	log "github.com/schollz/logger"
)

var flagDebug bool
var flagPort int

func init() {
	flag.IntVar(&flagPort, "port", 9222, "port for server")
	flag.BoolVar(&flagDebug, "debug", false, "debug mode")
}
func main() {
	flag.Parse()
	// use all of the processors
	runtime.GOMAXPROCS(runtime.NumCPU())
	if flagDebug {
		log.SetLevel("debug")
		log.Debug("debug mode")
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
		w.Header().Set("Access-Control-Allow-Origin", "*")
    		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
    		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		log.Debugf("opened %s %s", r.Method, r.URL.Path)
		defer func() {
			log.Debugf("finished %s\n", r.URL.Path)
		}()

		if r.URL.Path == "/" {
			// serve the README
			b, _ := ioutil.ReadFile("README.md")
			w.Write(b)
			return
		}
		if r.URL.Path=="/favicon.ico" {
			w.WriteHeader(http.StatusOK)
			return
		}

		_, doStream := r.URL.Query()["stream"]

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

			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Cache-Control", "no-cache, no-store")

			mimetyped := false
			canceled := false
			for {
				select {
				case s := <-channel:
					if s.done {
						canceled = true
					} else {
						if !mimetyped {
							mimetyped = true
							mimetype := mimetype.Detect(s.b).String()
							if mimetype == "application/octet-stream" {
								ext := strings.TrimPrefix(filepath.Ext(r.URL.Path), ".")
								log.Debug("checking extension %s", ext)
								mimetype = filetype.GetType(ext).MIME.Value
							}
							w.Header().Set("Content-Type", mimetype)
							log.Debugf("serving as Content-Type: '%s'", mimetype)
						}
						w.Write(s.b)
						w.(http.Flusher).Flush()
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
			cancel := true
			isdone := false
			lifetime := 0
			for {
				if !doStream {
					select {
					case <-r.Context().Done():
						isdone = true
					default:
					}
					if isdone {
						log.Debug("is done")
						break
					}
					mutex.Lock()
					numListeners := len(channels[r.URL.Path])
					mutex.Unlock()
					if numListeners == 0 {
						time.Sleep(1 * time.Second)
						lifetime++
						if lifetime > 600 {
							isdone = true
						}
						continue
					}
				}
				n, err := r.Body.Read(buffer)
				if err != nil {
					log.Debugf("err: %s", err)
					if err == io.ErrUnexpectedEOF {
						cancel = false
					}
					break
				}
				mutex.Lock()
				channels_current := channels[r.URL.Path]
				mutex.Unlock()
				for _, c := range channels_current {
					var b2 = make([]byte, n)
					copy(b2, buffer[:n])
					c <- stream{b: b2}
				}
			}
			if cancel {
				mutex.Lock()
				channels_current := channels[r.URL.Path]
				mutex.Unlock()
				for _, c := range channels_current {
					c <- stream{done: true}
				}
			}
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}

	log.Infof("running on port %d", flagPort)
	err = http.ListenAndServe(fmt.Sprintf(":%d", flagPort), http.HandlerFunc(handler))
	if err != nil {
		log.Error(err)
	}
	return
}
