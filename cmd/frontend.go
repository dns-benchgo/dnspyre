package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skratchdot/open-golang/open"
)

// FrontendConfig holds configuration for the frontend server
type FrontendConfig struct {
	Port        string
	Host        string
	OpenBrowser bool
	OutputFile  string
	PreloadFile string
}

// StartFrontendServer starts the web frontend server
func StartFrontendServer(config FrontendConfig) error {
	fs := GetFrontendFileSystem()
	if fs == nil {
		return fmt.Errorf("frontend filesystem not available")
	}

	// Load preload data if specified
	var preloadData string
	if config.PreloadFile != "" {
		data, err := os.ReadFile(config.PreloadFile)
		if err != nil {
			return fmt.Errorf("failed to read preload file %s: %v", config.PreloadFile, err)
		}
		preloadData = string(data)
		log.Printf("Preloaded data from: %s", config.PreloadFile)
	}

	mux := http.NewServeMux()

	// Serve static files
	fileServer := http.FileServer(fs)
	mux.Handle("/", fileServer)

	// API endpoint for getting preloaded data
	mux.HandleFunc("/api/preload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")

		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if preloadData == "" {
			w.Write([]byte(`{"data": null}`))
		} else {
			fmt.Fprintf(w, `{"data": %s}`, preloadData)
		}
	})

	// API endpoint for uploading benchmark results
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Handle file upload logic here
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	})

	// API endpoint for batch DNS server testing
	mux.HandleFunc("/api/batch-test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// TODO: Implement batch testing logic
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "batch test started", "message": "Batch testing feature coming soon"}`))
	})

	// API endpoint for comparing results
	mux.HandleFunc("/api/compare", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// TODO: Implement results comparison logic
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "comparison ready", "message": "Results comparison feature coming soon"}`))
	})

	// API endpoint for getting server rankings
	mux.HandleFunc("/api/rankings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// TODO: Implement server rankings logic
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "rankings": [], "message": "Server rankings feature coming soon"}`))
	})

	addr := config.Host + ":" + config.Port
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting dnspyre web frontend on http://%s", addr)
		if config.OpenBrowser {
			// Wait a moment for server to start, then open browser
			time.AfterFunc(time.Second, func() {
				url := fmt.Sprintf("http://%s", addr)
				if err := open.Run(url); err != nil {
					log.Printf("Failed to open browser: %v", err)
				}
			})
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down frontend server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown error: %v", err)
	}

	log.Println("Frontend server stopped")
	return nil
}
