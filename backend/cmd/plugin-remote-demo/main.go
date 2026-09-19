// Local protocol fixture for P04 acceptance. This server does not call models.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type job struct {
	ID        string
	Polls     int
	Cancelled bool
}

func main() {
	listen := flag.String("listen", "127.0.0.1:18081", "local demo listen address")
	flag.Parse()
	var mu sync.Mutex
	jobs := map[string]*job{}
	keys := map[string]string{}
	submits := 0
	var pngData bytes.Buffer
	preview := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			preview.Set(x, y, color.RGBA{R: 40, G: 120, B: 180, A: 255})
		}
	}
	if err := png.Encode(&pngData, preview); err != nil {
		log.Fatal(err)
	}
	write := func(w http.ResponseWriter, value any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /image.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData.Bytes())
	})
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]any{"message": "P04 同步测试完成（未调用模型）"})
	})
	mux.HandleFunc("POST /jobs", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Prompt string `json:"prompt"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&input) != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			http.Error(w, "idempotency key required", 400)
			return
		}
		mu.Lock()
		id, existing := keys[key]
		if !existing {
			submits++
			id = fmt.Sprintf("demo-%d", submits)
			keys[key] = id
			jobs[id] = &job{ID: id}
		}
		mu.Unlock()
		if !existing && strings.Contains(input.Prompt, "disconnect") {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		write(w, map[string]any{"id": id, "status": "queued"})
	})
	mux.HandleFunc("GET /requests/{key}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		id := keys[r.PathValue("key")]
		mu.Unlock()
		if id == "" {
			http.NotFound(w, r)
			return
		}
		write(w, map[string]any{"id": id, "status": "queued"})
	})
	mux.HandleFunc("GET /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		j := jobs[r.PathValue("id")]
		if j == nil {
			mu.Unlock()
			http.NotFound(w, r)
			return
		}
		j.Polls++
		state := "running"
		if j.Cancelled {
			state = "cancelled"
		} else if j.Polls >= 2 {
			state = "completed"
		}
		id := j.ID
		mu.Unlock()
		write(w, map[string]any{"id": id, "status": state, "output": map[string]any{"message": "P04 异步测试产物（固定测试图片，未调用模型）", "url": "http://" + r.Host + "/image.png"}})
	})
	mux.HandleFunc("POST /jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		j := jobs[r.PathValue("id")]
		if j != nil {
			j.Cancelled = true
		}
		mu.Unlock()
		if j == nil {
			http.NotFound(w, r)
			return
		}
		write(w, map[string]any{"status": "running"})
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count := submits
		mu.Unlock()
		write(w, map[string]any{"uniqueSubmissions": count, "demo": true})
	})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/image.png" && r.Header.Get("Authorization") != "Bearer demo-key" {
			http.Error(w, "demo authentication required", 401)
			return
		}
		mux.ServeHTTP(w, r)
	})
	server := http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	log.Printf("P04 local demo at http://%s; test credential: demo-key; no model calls", *listen)
	log.Fatal(server.ListenAndServe())
}
