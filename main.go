package main

import (
	"flag"
	"fmt"
	"html/template"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/h2non/filetype"
	log "github.com/schollz/logger"
)

var flagDebug bool
var flagPort int
var flagFolder string

func init() {
	flag.StringVar(&flagFolder, "folder", "archived", "folder to save archived")
	flag.IntVar(&flagPort, "port", 9222, "port for server")
	flag.BoolVar(&flagDebug, "debug", false, "debug mode")
}

func main() {
	flag.Parse()
	os.MkdirAll(flagFolder, os.ModePerm)
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
	archived := make(map[string]*os.File)
	advertisements := make(map[string]bool)
	mutex := &sync.Mutex{}
	const tpl = `
	<!DOCTYPE html>
	<html>
		<head>
			<meta charset="UTF-8">
			<meta name="viewport" content="width=device-width, initial-scale=1">
			<title>{{.Title}}</title>
			<style type="text/css">body{margin:40px
				auto;max-width:650px;line-height:1.6;font-size:18px;padding:0
				10px}h1,h2,h3{line-height:1.2}
				.delete {
					color:#000;
					text-decoration:none;
				}

				
summary {
	white-space: nowrap;
 }
 
 details > summary {
   list-style: none;
 }
 details > summary::-webkit-details-marker {
   display: none;
 }
 
 details {
   display: inline;
 }
 
 details[open] {
	 padding: 1em;
	 display: block;
 }
 
			</style>
		</head>
		<body>
				<h1>Current broadcasts:</h1>
				{{range .Items}}<a href="/{{ . }}">{{ . }}</a><br> <audio controls preload="none">
					<source src="/{{ . }}?r={{$.Rand}}" type="audio/mpeg">
					Your browser does not support the audio element.
				</audio><br><br>
			  {{else}}<div><strong>No broadcasts currently.</strong></div>{{end}}
				<h2>Archived broadcasts:</h2>
				{{if .Archived}}<p><small>click ❌ to remove an archive (<em>maybe don't remove ones that you didn't create</em>).</small></p>{{end}}
				{{range .Archived}}<a href="/{{ .FullFilename }}">{{ .Filename }}</a> <small>({{.Created.Format "Jan 02, 2006 15:04:05 UTC"}}, 
				<details><summary>❌</summary>are you sure? <details><summary>click->👍</summary>absolutely sure? <a class="delete" href="/{{ .FullFilename }}?remove=true">🗑️</a></details></details>)
				</small><br> <audio controls preload="none">
					<source src="/{{ .FullFilename }}?r={{$.Rand}}" type="audio/mpeg">
					Your browser does not support the audio element.
				</audio><br><br>
			  {{else}}<div><strong>No archives currently.</strong></div>{{end}}
			
		</body>
	</html>`
	tplmain, err := template.New("webpage").Parse(tpl)
	if err != nil {
		return
	}

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
			adverts := []string{}
			mutex.Lock()
			for advert := range advertisements {
				adverts = append(adverts, strings.TrimPrefix(advert, "/"))
			}
			mutex.Unlock()

			active := make(map[string]struct{})
			// mutex.Lock()
			// for ch := range channels {
			// 	active[strings.TrimPrefix(ch, "/")] = struct{}{}
			// }
			// log.Debugf("active: %+v", active)
			// mutex.Unlock()

			data := struct {
				Title    string
				Items    []string
				Rand     string
				Archived []ArchivedFile
			}{
				Title:    "Current broadcasts",
				Items:    adverts,
				Rand:     fmt.Sprintf("%d", rand.Int31()),
				Archived: listArchived(active),
			}
			err = tplmain.Execute(w, data)
			return
		} else if r.URL.Path == "/favicon.ico" {
			w.WriteHeader(http.StatusOK)
			return
		} else if strings.HasPrefix(r.URL.Path, "/"+flagFolder+"/") {
			filename := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"+flagFolder+"/"))
			// This extra join implicitly does a clean and thereby prevents directory traversal
			filename = path.Join("/", filename)
			filename = path.Join(flagFolder, filename)
			v, ok := r.URL.Query()["remove"]
			if ok && v[0] == "true" {
				os.Remove(filename)
				w.Write([]byte(fmt.Sprintf("removed %s", filename)))
			} else {
				http.ServeFile(w, r, filename)
			}
			return
		}

		v, ok := r.URL.Query()["stream"]
		doStream := ok && v[0] == "true"

		v, ok = r.URL.Query()["archive"]
		doArchive := ok && v[0] == "true"

		if doArchive && r.Method == "POST" {
			if _, ok := archived[r.URL.Path]; !ok {
				folderName := path.Join(flagFolder, time.Now().Format("200601021504"))
				os.MkdirAll(folderName, os.ModePerm)
				archived[r.URL.Path], err = os.Create(path.Join(folderName, strings.TrimPrefix(r.URL.Path, "/")))
				if err != nil {
					log.Error(err)
				}
			}
			defer func() {
				mutex.Lock()
				if _, ok := archived[r.URL.Path]; ok {
					log.Debugf("closed archive for %s", r.URL.Path)
					archived[r.URL.Path].Close()
					delete(archived, r.URL.Path)
				}
				mutex.Unlock()
			}()
		}

		v, ok = r.URL.Query()["advertise"]
		if ok && v[0] == "true" && doStream {
			mutex.Lock()
			advertisements[r.URL.Path] = true
			mutex.Unlock()
			defer func() {
				mutex.Lock()
				delete(advertisements, r.URL.Path)
				mutex.Unlock()
			}()
		}

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
			delete(channels[r.URL.Path], id)
			log.Debugf("removed listener %f", id)
			mutex.Unlock()
			close(channel)
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
				if doArchive {
					mutex.Lock()
					archived[r.URL.Path].Write(buffer[:n])
					mutex.Unlock()
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

type ArchivedFile struct {
	Filename     string
	FullFilename string
	Created      time.Time
}

func listArchived(active map[string]struct{}) (afiles []ArchivedFile) {
	fnames := []string{}
	err := filepath.Walk(flagFolder,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				fnames = append(fnames, path)
			}
			return nil
		})
	if err != nil {
		return
	}
	for _, fname := range fnames {
		_, onlyfname := path.Split(fname)
		finfo, _ := os.Stat(fname)
		stat_t := finfo.Sys().(*syscall.Stat_t)
		created := timespecToTime(stat_t.Ctim)
		if _, ok := active[onlyfname]; !ok {
			afiles = append(afiles, ArchivedFile{
				Filename:     onlyfname,
				FullFilename: fname,
				Created:      created,
			})
		}
	}

	sort.Slice(afiles, func(i, j int) bool {
		return afiles[i].Created.After(afiles[j].Created)
	})

	return
}

func timespecToTime(ts syscall.Timespec) time.Time {
	return time.Unix(int64(ts.Sec), int64(ts.Nsec))
}
