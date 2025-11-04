package config

import (
	"context"
	"encoding/base64"
	"log"
	"os"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

var FirebaseApp *firebase.App

// InitFirebase initializes the Firebase Admin SDK
func InitFirebase() {
	ctx := context.Background()

	// Check for base64 encoded credentials first
	if base64Creds := os.Getenv("FIREBASE_CREDENTIALS_BASE64"); base64Creds != "" {
		log.Printf("Using Firebase credentials from base64 environment variable")
		decoded, err := base64.StdEncoding.DecodeString(base64Creds)
		if err != nil {
			log.Fatalf("Error decoding base64 credentials: %v", err)
		}

		opt := option.WithCredentialsJSON(decoded)
		config := &firebase.Config{
			ProjectID: "barrim-93482",
		}

		app, err := firebase.NewApp(ctx, config, opt)
		if err != nil {
			log.Fatalf("error initializing firebase app: %v\n", err)
		}
		FirebaseApp = app
		return
	}

	// Fallback to file-based credentials
	credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if credFile == "" {
		// Try multiple possible locations
		possiblePaths := []string{
			"barrim-93482-firebase-adminsdk-fbsvc-4ee236d155.json",
			"../barrim-93482-firebase-adminsdk-fbsvc-4ee236d155.json",
			"./barrim-93482-firebase-adminsdk-fbsvc-4ee236d155.json",
		}

		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				credFile = path
				break
			}
		}

		if credFile == "" {
			log.Fatalf("Firebase service account file not found. Please set GOOGLE_APPLICATION_CREDENTIALS environment variable, FIREBASE_CREDENTIALS_BASE64, or place the file in one of these locations: %v", possiblePaths)
		}
	}

	log.Printf("Using Firebase credentials file: %s", credFile)
	opt := option.WithCredentialsFile(credFile)

	// Create Firebase config with project ID
	config := &firebase.Config{
		ProjectID: "barrim-93482",
	}

	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		log.Fatalf("error initializing firebase app: %v\n", err)
	}
	FirebaseApp = app
}
