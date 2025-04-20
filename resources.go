package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

//go:embed assets/*
var embeddedAssets embed.FS

// extractEmbeddedAssets extracts the embedded assets to the application data directory
func extractEmbeddedAssets(appDataDir string) error {
	log.Println("Extracting embedded assets to:", appDataDir)
	assetsDir := filepath.Join(appDataDir, "assets")
	err := os.MkdirAll(assetsDir, 0755)
	if err != nil {
		return err
	}

	return fs.WalkDir(embeddedAssets, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			// Read the file
			data, err := embeddedAssets.ReadFile(path)
			if err != nil {
				log.Printf("Error reading embedded asset %s: %v", path, err)
				return err
			}

			// Create the file in the app data directory
			targetPath := filepath.Join(appDataDir, path)
			targetDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				log.Printf("Error creating directory for %s: %v", targetPath, err)
				return err
			}

			if err := os.WriteFile(targetPath, data, 0644); err != nil {
				log.Printf("Error writing extracted asset %s: %v", targetPath, err)
				return err
			}
			log.Printf("Extracted asset: %s", targetPath)
		}
		return nil
	})
}

func getIconPath() (string, error) {
	iconBytes, err := embeddedAssets.ReadFile("assets/icon.ico")
	if err != nil {
		return "", err
	}

	// Write to temp file since walk.NewIconFromFile needs a file path
	tmpFile := filepath.Join(os.TempDir(), "app_icon.ico")
	if err := os.WriteFile(tmpFile, iconBytes, 0644); err != nil {
		return "", err
	}

	return tmpFile, nil
}
