package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	// This import path references your actual module's path + "/utils".
	// Example: if your module name is "github.com/opengovern/resilient-bridge"
	// then the path below is correct. Adjust as needed for your environment.
)

func main() {
	var filePath string
	flag.StringVar(&filePath, "file", "", "Path to Dockerfile")
	flag.Parse()

	if filePath == "" {
		log.Fatal("Usage: go run main.go --file=./Dockerfile")
	}

	// 1) Read the Dockerfile from disk
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read Dockerfile: %v", err)
	}

	// 2) Encode content in base64
	encoded := base64.StdEncoding.EncodeToString(content)

	// 3) Call the function from the 'utils' package to parse the base64 Dockerfile and collect base images
	images, err := utils.ExtractExternalBaseImagesFromBase64(encoded)
	if err != nil {
		log.Fatalf("Error extracting base images: %v", err)
	}

	// 4) Print the result
	fmt.Println("External base images found:")
	for i, img := range images {
		fmt.Printf("%d) %s\n", i+1, img)
	}
}
