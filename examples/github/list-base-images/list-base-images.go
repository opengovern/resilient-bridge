package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	// Import the 'utils' package from your repository
	utils "github.com/opengovern/resilient-bridge/utils"
)

func main() {
	var filePath string
	flag.StringVar(&filePath, "file", "", "Path to Dockerfile")
	flag.Parse()

	if filePath == "" {
		log.Fatal("Usage: go run list-base-images.go --file=./Dockerfile")
	}

	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read Dockerfile at %s: %v", filePath, err)
	}

	encoded := base64.StdEncoding.EncodeToString(content)

	// Call the function from the utils package
	images, err := utils.ExtractExternalBaseImagesFromBase64(encoded)
	if err != nil {
		log.Fatalf("Error extracting base images: %v", err)
	}

	fmt.Println("External base images found:")
	for i, img := range images {
		fmt.Printf("%d) %s\n", i+1, img)
	}
}
