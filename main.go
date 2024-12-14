package main

import (
	"context"
	"log"
	"os"

	firebaseAdmin "firebase.google.com/go/v4"
	api "github.com/dolater/dolater-internal-api/generated"
	"github.com/dolater/dolater-internal-api/middleware"
	"github.com/dolater/dolater-internal-api/server"
	"github.com/joho/godotenv"

	"github.com/gin-gonic/gin"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}

	if os.Getenv("MODE") != "DEBUG" {
		os.Setenv("GIN_MODE", "release")
	}

	// Firebase
	app, err := firebaseAdmin.NewApp(context.Background(), nil)
	if err != nil {
		log.Printf("Error initializing app: %v\n", err)
	}

	r := gin.Default()

	// Middleware
	m := middleware.New(app)
	if os.Getenv("MODE") != "DEBUG" {
		r.Use(m.RequireAppCheck())
	}
	r.Use(m.GetFirebaseAuthIDToken())

	// Server
	s := server.New(app)
	api.RegisterHandlers(r, s)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	r.Run(":" + port)
}
