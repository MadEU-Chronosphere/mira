package main

import (
	"chronosphere/bootstrap"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	fmt.Println("=====================================")
	fmt.Println("🚀 Starting Chronosphere Myra Backend Application")
	fmt.Println("=====================================")
	// Initialize Application
	// Parse flags
	mode := flag.String("mode", "full", "Application mode: full, no-wa, no-limiter")
	flag.Parse()

	var app *gin.Engine

	switch *mode {
	case "no-wa":
		log.Println("🚀 Starting in NO-WA mode")
		app, _ = bootstrap.InitializeAppWithoutWhatsappNotification()
	case "no-limiter":
		log.Println("🚀 Starting in NO-LIMITER mode")
		app, _ = bootstrap.InitializeAppWithoutRateLimiter()
	default:
		log.Println("🚀 Starting in FULL mode")
		app, _ = bootstrap.InitializeFullApp()
	}

	// ========================================================================
	// GRACEFUL SHUTDOWN SETUP
	// ========================================================================
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srvAddr := ":" + port

	// Create HTTP server with custom configuration
	srv := &http.Server{
		Addr:           srvAddr,
		Handler:        app,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Start server in a goroutine
	go func() {
		log.Printf("🚀 Server running at http://localhost%s", srvAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Server error: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down server...")

	// The context is used to inform the server it has 10 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("⚠️  Server forced to shutdown: %v", err)
	}

	log.Println("✅ Server exited gracefully")
}
