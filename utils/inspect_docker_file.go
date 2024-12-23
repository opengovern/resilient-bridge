// File: utils/inspect_docker_file.go

package utils

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// fromInfo holds info about a FROM line (the base image and any alias).
type fromInfo struct {
	baseImage string
	alias     string
}

// extractExternalBaseImagesFromBase64 decodes the Dockerfile from base64,
// parses it with BuildKit, collects base images, expands ARG references, and
// filters out internal (stage) aliases. Returns a list of "external" images.
//
// Usage:
//
//	images, err := extractExternalBaseImagesFromBase64(encoded)
//	if err != nil { ... }
//	for _, img := range images {
//	    fmt.Println("Found base image:", img)
//	}
func extractExternalBaseImagesFromBase64(encodedDockerfile string) ([]string, error) {
	// --- A: Decode base64 Dockerfile content ---
	decoded, err := base64.StdEncoding.DecodeString(encodedDockerfile)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode Dockerfile: %w", err)
	}

	// --- B: Parse with Docker BuildKit parser ---
	res, err := parser.Parse(strings.NewReader(string(decoded)))
	if err != nil {
		return nil, fmt.Errorf("BuildKit parser error: %w", err)
	}
	if res == nil || res.AST == nil {
		return nil, nil // no AST to process
	}

	// --- C: Collect top-level ARG instructions for naive variable expansion ---
	argsMap := collectArgs(res.AST)

	// --- D: Gather FROM instructions, expand them
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

	// --- E: Filter out references to internal aliases
	var external []string
	for _, f := range fromLines {
		// If the baseImage is itself a known alias, skip it
		if stageAliases[f.baseImage] {
			continue
		}
		external = append(external, f.baseImage)
	}

	return external, nil
}

// collectArgs gathers top-level ARG instructions from the AST.
// We do naive parsing for ARG lines like: ARG KEY=VALUE
// If the Dockerfile has e.g. "ARG MYARG=ubuntu:20.04" we store that in a map
// for naive expansion: ${MYARG} => ubuntu:20.04
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

// collectStatementTokens flattens tokens for a single statement, stopping if we
// hit another Dockerfile instruction. This mirrors typical Dockerfile grammar.
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

// parseFromLine processes tokens from a FROM statement,
// e.g. ["--platform=${PLATFORM}", "${GO_IMAGE}", "AS", "builder"] -> (baseImage, alias).
func parseFromLine(tokens []string, argsMap map[string]string) (string, string) {
	var base, alias string
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if strings.HasPrefix(t, "--") {
			// e.g. --platform=...
			continue
		}
		// "AS builder" => capture builder as alias
		if strings.EqualFold(t, "AS") && i+1 < len(tokens) {
			alias = tokens[i+1]
			break
		}
		// First non -- token we treat as the image reference (possibly containing $VAR)
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

// expandArgs does naive expansion of $VAR or ${VAR} with known defaults in argsMap.
func expandArgs(input string, argsMap map[string]string) string {
	return os.Expand(input, func(key string) string {
		if val, ok := argsMap[key]; ok {
			return val
		}
		// If we have no known substitution, keep it as is:
		// e.g. input had ${SOMEVAR}, we can't expand => return ${SOMEVAR}
		return fmt.Sprintf("${%s}", key)
	})
}

// isInstructionKeyword checks if a token is recognized as a Dockerfile instruction.
func isInstructionKeyword(s string) bool {
	switch strings.ToUpper(s) {
	case "ADD",
		"ARG",
		"CMD",
		"COPY",
		"ENTRYPOINT",
		"ENV",
		"EXPOSE",
		"FROM",
		"HEALTHCHECK",
		"LABEL",
		"MAINTAINER",
		"ONBUILD",
		"RUN",
		"SHELL",
		"STOPSIGNAL",
		"USER",
		"VOLUME",
		"WORKDIR":
		return true
	}
	return false
}
