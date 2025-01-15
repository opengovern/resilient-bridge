// file: resilient-bridge/utils/inspect_docker_file.go

package utils

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// ExtractExternalBaseImagesFromBase64 parses a base64-encoded Dockerfile, expands ARG references,
// and returns a deduplicated slice of external (non-alias) base images. If an ARG is used but never
// defined, we treat it as an unknown version (e.g. "fedora:*").
func ExtractExternalBaseImagesFromBase64(encodedDockerfile string) ([]string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encodedDockerfile)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 content: %w", err)
	}

	// Parse with BuildKit
	res, err := parser.Parse(strings.NewReader(string(decoded)))
	if err != nil {
		return nil, fmt.Errorf("BuildKit parser error: %w", err)
	}
	if res == nil || res.AST == nil {
		return nil, nil
	}

	// 1. Collect ARG key-value defaults:
	argsMap := collectArgs(res.AST)

	// 2. Gather FROM lines (base image + optional alias)
	var froms []fromInfo
	stageAliases := make(map[string]bool)
	for _, stmt := range res.AST.Children {
		if strings.EqualFold(stmt.Value, "from") {
			tokens := collectStatementTokens(stmt)
			base, alias := parseFromLine(tokens, argsMap)
			if alias != "" {
				stageAliases[alias] = true
			}
			froms = append(froms, fromInfo{baseImage: base, alias: alias})
		}
	}

	// 3. Filter out references to internal aliases (e.g. FROM builder)
	var allBaseImages []string
	for _, f := range froms {
		if stageAliases[f.baseImage] {
			continue
		}
		allBaseImages = append(allBaseImages, f.baseImage)
	}

	// 4. Deduplicate base images
	return deduplicateStrings(allBaseImages), nil
}

// fromInfo holds (baseImage, alias) for one FROM line.
type fromInfo struct {
	baseImage string
	alias     string
}

// deduplicateStrings returns a slice of unique strings, preserving order.
func deduplicateStrings(input []string) []string {
	seen := make(map[string]bool, len(input))
	var result []string
	for _, v := range input {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

// collectArgs scans all top-level `ARG` instructions to fill a map of known defaults.
// e.g. `ARG PLUGIN_REGISTRY=opengovern` => argsMap["PLUGIN_REGISTRY"] = "opengovern".
func collectArgs(ast *parser.Node) map[string]string {
	argsMap := make(map[string]string)
	for _, stmt := range ast.Children {
		if strings.EqualFold(stmt.Value, "arg") {
			tokens := collectStatementTokens(stmt)
			for _, t := range tokens {
				k, v := parseArgKeyValue(t)
				if k != "" {
					argsMap[k] = v // could be empty if ARG is declared but not assigned
				}
			}
		}
	}
	return argsMap
}

// parseArgKeyValue splits "KEY=VALUE". If no '=', we get (KEY, "").
func parseArgKeyValue(s string) (string, string) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// collectStatementTokens returns tokens for the statement, stopping on the next instruction.
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

// parseFromLine processes tokens from a FROM statement, e.g. ["--platform=${PLATFORM}", "${GO_IMAGE}", "AS", "builder"].
//
// We do a naive expansion: if an ARG is never defined, we fallback to "repo:*" for that base image.
// Example: "FROM fedora:$FEDORA_VERSION" with no default => "fedora:*".
func parseFromLine(tokens []string, args map[string]string) (string, string) {
	var base, alias string
	var encounteredUndefinedArg bool

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]

		// e.g. --platform=linux/amd64
		if strings.HasPrefix(t, "--") {
			continue
		}

		// If we see "AS builder", the next token is the alias
		if strings.EqualFold(t, "AS") && i+1 < len(tokens) {
			alias = tokens[i+1]
			break
		}

		// Expand references
		expanded := os.Expand(t, func(k string) string {
			if v, ok := args[k]; ok {
				return v
			}
			// If ARG not defined, note it
			encounteredUndefinedArg = true
			return ""
		})

		// The first token that isn't a flag or AS is the base image
		if base == "" {
			base = expanded
		}
	}

	// If any undefined ARG was encountered, fallback to "repo:*"
	if encounteredUndefinedArg && base != "" {
		base = fallbackRepoTag(base)
	}

	return base, alias
}

// fallbackRepoTag replaces everything after the first colon with "*", or
// if no colon is present, appends ":*". This ensures we produce something
// like "fedora:*" or "ghcr.io/myorg/fedora:*" if the ARG-based tag was
// unresolved.
func fallbackRepoTag(imageRef string) string {
	if !strings.Contains(imageRef, ":") {
		return imageRef + ":*"
	}
	parts := strings.SplitN(imageRef, ":", 2)
	return parts[0] + ":*"
}

// isInstructionKeyword checks if a token is recognized as a Dockerfile instruction.
func isInstructionKeyword(s string) bool {
	switch strings.ToUpper(s) {
	case "ADD", "ARG", "CMD", "COPY", "ENTRYPOINT", "ENV", "EXPOSE",
		"FROM", "HEALTHCHECK", "LABEL", "MAINTAINER", "ONBUILD",
		"RUN", "SHELL", "STOPSIGNAL", "USER", "VOLUME", "WORKDIR":
		return true
	}
	return false
}
