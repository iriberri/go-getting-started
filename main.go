package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/heroku/x/hmetrics/onload"
	_ "github.com/lib/pq"
)

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	// Initialize database connection
	db, err := initDB()
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
	} else if db != nil {
		defer db.Close()
		log.Println("Database connection established successfully")
	} else {
		log.Println("No DATABASE_URL configured, skipping database setup")
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.LoadHTMLGlob("templates/*.tmpl.html")
	router.Static("/static", "static")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl.html", nil)
	})

	// Database health check endpoint
	router.GET("/db", func(c *gin.Context) {
		if db == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "Database not configured"})
			return
		}

		var result int
		err := db.QueryRow("SELECT 1").Scan(&result)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "result": result})
	})

	router.Run(":" + port)
}

func initDB() (*sql.DB, error) {
	// PGBouncer sets DATABASE_URL automatically
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Println("DATABASE_URL not set")
		return nil, nil
	}

	// When using PGBouncer, the connection is local (127.0.0.1:6000)
	// and should not use SSL. PGBouncer handles SSL to the actual database.
	if strings.Contains(databaseURL, "127.0.0.1:6000") {
		if !strings.Contains(databaseURL, "sslmode=") {
			if strings.Contains(databaseURL, "?") {
				databaseURL += "&sslmode=disable"
			} else {
				databaseURL += "?sslmode=disable"
			}
			log.Println("Added sslmode=disable for PGBouncer local connection")
		}
	}

	log.Printf("Attempting to connect to database (URL length: %d chars)", len(databaseURL))

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Printf("ERROR in sql.Open: %v", err)
		return nil, err
	}

	// Test the connection with retry for PGBouncer startup
	log.Println("Testing database connection with Ping...")
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		err = db.Ping()
		if err == nil {
			log.Println("Successfully connected to database")
			return db, nil
		}

		log.Printf("Ping attempt %d/%d failed: %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			time.Sleep(2 * time.Second)
		}
	}

	log.Printf("ERROR: Failed to connect after %d attempts: %v", maxRetries, err)
	return nil, err
}
