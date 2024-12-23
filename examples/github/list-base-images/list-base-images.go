package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// main shows how to:
// 1) Read a Dockerfile from disk and convert it to base64.
// 2) Call extractExternalBaseImagesFromBase64(...) to parse it and return the base images.
func main() {
	var filePath string
	flag.StringVar(&filePath, "file", "", "Path to Dockerfile")
	flag.Parse()
	if filePath == "" {
		log.Fatal("Usage: go run main.go --file=./Dockerfile")
	}

	// 1) Read the Dockerfile
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read Dockerfile: %v", err)
	}

	// 2) Encode content in base64
	encoded := base64.StdEncoding.EncodeToString(content)

	// 3) Call our single function to parse the base64 Dockerfile and collect base images
	images, err := extractExternalBaseImagesFromBase64(encoded)
	if err != nil {
		log.Fatalf("Error extracting base images: %v", err)
	}

	// 4) Print the result
	fmt.Println("External base images found:")
	for i, img := range images {
		fmt.Printf("%d) %s\n", i+1, img)
	}
}

// extractExternalBaseImagesFromBase64 takes a base64-encoded Dockerfile and returns
// a slice of external base images, skipping references to prior stages (aliases).
func extractExternalBaseImagesFromBase64(encodedDockerfile string) ([]string, error) {
	// --- STEP A: Decode the base64 Dockerfile content ---
	decoded, err := base64.StdEncoding.DecodeString(encodedDockerfile)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode Dockerfile: %w", err)
	}

	// --- STEP B: Parse the Dockerfile with the BuildKit parser ---
	// We can parse from a memory buffer by wrapping decoded content in strings.NewReader.
	res, err := parser.Parse(strings.NewReader(string(decoded)))
	if err != nil {
		return nil, fmt.Errorf("BuildKit parser error: %w", err)
	}

	// --- STEP C: Collect all ARG instructions for naive variable expansion ---
	argsMap := collectArgs(res.AST)

	// --- STEP D: Walk each top-level statement, gather FROM instructions ---
	var fromLines []fromInfo
	stageAliases := make(map[string]bool)
	for _, stmt := range res.AST.Children {
		if strings.EqualFold(stmt.Value, "from") {
			tokens := collectStatementTokens(stmt)
			base, alias := parseFromLine(tokens, argsMap)
			if alias != "" {
				stageAliases[alias] = true
			}
			fromLines = append(fromLines, fromInfo{baseImage: base, alias: alias})
		}
	}

	// --- STEP E: Filter out references to internal aliases ---
	var external []string
	for _, f := range fromLines {
		// If the "baseImage" is itself a known alias, skip it
		if stageAliases[f.baseImage] {
			continue
		}
		external = append(external, f.baseImage)
	}
	return external, nil
}

// fromInfo holds the result of parsing one FROM instruction
type fromInfo struct {
	baseImage string
	alias     string
}

// --- Utility functions ---

// collectArgs finds all top-level ARG instructions and captures
// e.g. "ARG GO_IMAGE=golang:1.23" => argsMap["GO_IMAGE"] = "golang:1.23"
func collectArgs(ast *parser.Node) map[string]string {
	argsMap := make(map[string]string)
	for _, stmt := range ast.Children {
		if strings.EqualFold(stmt.Value, "arg") {
			tokens := collectStatementTokens(stmt)
			for _, t := range tokens {
				k, v := parseArgKeyValue(t)
				if k != "" && v != "" {
					argsMap[k] = v
				}
			}
		}
	}
	return argsMap
}

// collectStatementTokens flattens the chain of tokens for a statement,
// stopping if we hit another Dockerfile instruction (FROM, RUN, COPY, etc.)
func collectStatementTokens(stmt *parser.Node) []string {
	var tokens []string
	cur := stmt.Next
	for cur != nil {
		if isInstructionKeyword(cur.Value) {
			break
		}
		tokens = append(tokens, cur.Value)
		cur = cur.Next
	}
	return tokens
}

// parseFromLine processes tokens from a FROM statement, e.g.
//
//	["--platform=${JS_PLATFORM}", "${GO_IMAGE}", "AS", "builder"]
//
// returns (baseImage, alias).
func parseFromLine(tokens []string, argsMap map[string]string) (string, string) {
	var base, alias string
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if strings.HasPrefix(t, "--") {
			// e.g. --platform=...
			continue
		}
		if strings.EqualFold(t, "AS") && i+1 < len(tokens) {
			alias = tokens[i+1]
			break
		}
		// This is presumably our base image reference, e.g. "golang:1.23" or "${GO_IMAGE}"
		base = expandArgs(t, argsMap)
	}
	return base, alias
}

// parseArgKeyValue splits "KEY=VALUE". If no '=', returns (KEY, "").
func parseArgKeyValue(argToken string) (string, string) {
	parts := strings.SplitN(argToken, "=", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// expandArgs does a naive expansion of $VAR / ${VAR} with known defaults in argsMap.
// If no default is found, we leave it as ${VAR}.
func expandArgs(input string, argsMap map[string]string) string {
	return os.Expand(input, func(key string) string {
		if val, ok := argsMap[key]; ok {
			return val
		}
		return fmt.Sprintf("${%s}", key)
	})
}

// isInstructionKeyword checks if a token is recognized as a Dockerfile instruction.
// Add or remove from this list if needed for additional instructions.
func isInstructionKeyword(s string) bool {
	switch strings.ToUpper(s) {
	case "ADD", "ARG", "CMD", "COPY", "ENTRYPOINT", "ENV", "EXPOSE",
		"FROM", "HEALTHCHECK", "LABEL", "MAINTAINER", "ONBUILD",
		"RUN", "SHELL", "STOPSIGNAL", "USER", "VOLUME", "WORKDIR":
		return true
	}
	return false
}
