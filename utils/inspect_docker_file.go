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
// and returns a slice of external (non-alias) base images in the Dockerfile.
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

	// 2. Gather FROM lines
	var froms []fromInfo
	stageAliases := make(map[string]bool)
	for _, stmt := range res.AST.Children {
		if strings.EqualFold(stmt.Value, "from") {
			tokens := collectStatementTokens(stmt)
			base, alias := parseFromLine(tokens, argsMap)
			if alias != "" {
				stageAliases[alias] = true
			}
			froms = append(froms, fromInfo{base, alias})
		}
	}

	// 3. Filter out references to internal aliases
	var external []string
	for _, f := range froms {
		if stageAliases[f.baseImage] {
			// Means it's referencing a prior stage alias
			continue
		}
		external = append(external, f.baseImage)
	}
	return external, nil
}

// fromInfo holds (baseImage, alias) for one FROM line.
type fromInfo struct {
	baseImage string
	alias     string
}

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

// parseArgKeyValue splits "KEY=VALUE".
func parseArgKeyValue(s string) (string, string) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// collectStatementTokens returns tokens for the statement, stopping on next instruction
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

// parseFromLine processes tokens from FROM statement, e.g. ["--platform=${PLATFORM}", "${GO_IMAGE}", "AS", "builder"]
func parseFromLine(tokens []string, args map[string]string) (string, string) {
	var base, alias string
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if strings.HasPrefix(t, "--") {
			// e.g. --platform=linux/amd64
			continue
		}
		if strings.EqualFold(t, "AS") && i+1 < len(tokens) {
			alias = tokens[i+1]
			break
		}
		base = expandArgs(t, args)
	}
	return base, alias
}

// expandArgs replaces any ${VAR} references with known arg defaults from the map
func expandArgs(val string, args map[string]string) string {
	return os.Expand(val, func(k string) string {
		if v, ok := args[k]; ok {
			return v
		}
		return "${" + k + "}"
	})
}

func isInstructionKeyword(s string) bool {
	switch strings.ToUpper(s) {
	case "ADD", "ARG", "CMD", "COPY", "ENTRYPOINT", "ENV", "EXPOSE",
		"FROM", "HEALTHCHECK", "LABEL", "MAINTAINER", "ONBUILD",
		"RUN", "SHELL", "STOPSIGNAL", "USER", "VOLUME", "WORKDIR":
		return true
	}
	return false
}
