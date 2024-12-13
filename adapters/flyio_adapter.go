package adapters

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

const (
	FlyIOGetMachineRate = 5 // req/s for get_machine
	FlyIOOtherRate      = 1 // req/s for other actions

	// The doc mentions short-term burst, but we ignore that for simplicity.
)

// We'll use regex to identify endpoints and actions.
var machineIDPattern = regexp.MustCompile(`^/apps/[^/]+/machines/([^/]+)(/.*)?$`)

// FlyIOAdapter implements rate limiting per-action, per-machine for Fly.io Machines API.
type FlyIOAdapter struct {
	APIToken string

	mu             sync.Mutex
	requestHistory map[string][]int64 // key: action:machine_id or action:global
}

func NewFlyIOAdapter(apiToken string) *FlyIOAdapter {
	return &FlyIOAdapter{
		APIToken:       apiToken,
		requestHistory: make(map[string][]int64),
	}
}

func (f *FlyIOAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	// Fly.io has fixed rules, ignoring provider overrides for simplicity.
	// If needed, you can implement logic to store overrides.
}

func (f *FlyIOAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	action, machineID := f.classifyRequest(req)
	if f.isRateLimited(action, machineID) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Fly.io rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	baseURL := "https://api.machines.dev/v1"
	fullURL := baseURL + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Authorization") == "" && f.APIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+f.APIToken)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	f.recordRequest(action, machineID)

	data, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headers[strings.ToLower(k)] = vals[0]
		}
	}

	return &resilientbridge.NormalizedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Data:       data,
	}, nil
}

func (f *FlyIOAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	// The doc does not mention specific rate limit headers like X-RateLimit-Limit for Machines API.
	// If provided, parse them here. Otherwise, return nil.
	return nil, nil
}

func (f *FlyIOAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (f *FlyIOAdapter) classifyRequest(req *resilientbridge.NormalizedRequest) (action string, machineID string) {
	// Identify action from method and endpoint
	// Examples:
	// GET /apps/{app}/machines -> list_machines
	// GET /apps/{app}/machines/{machine_id} -> get_machine
	// POST /apps/{app}/machines -> create_machine
	// POST /apps/{app}/machines/{machine_id}/start -> start_machine
	// We'll guess action by last path segment if known, or by method if unknown.

	path := req.Endpoint
	method := strings.ToUpper(req.Method)

	if method == "GET" {
		// Check if it is /apps/{app}/machines or /apps/{app}/machines/xxx
		if machineIDMatch := machineIDPattern.FindStringSubmatch(path); machineIDMatch != nil {
			// This is a GET machine
			action = "get_machine"
			machineID = machineIDMatch[1]
			return action, machineID
		} else {
			// Likely listing machines
			action = "list_machines"
			// No machine_id for listing
			return action, ""
		}
	} else {
		// Non-GET methods
		// Check if machine_id is present
		if machineIDMatch := machineIDPattern.FindStringSubmatch(path); machineIDMatch != nil {
			// Action depends on the last segment
			parts := strings.Split(path, "/")
			last := parts[len(parts)-1]
			if last == machineIDMatch[1] {
				// something like POST /apps/app/machines/<id> might be "update_machine" or "start_machine"
				// Not specified. We'll guess action by method + something:
				// Without official mapping, just say method+machine: "update_machine" for PUT/PATCH, "delete_machine" for DELETE,
				// "start_machine" if endpoint ends with "/start" etc.
				// For simplicity:
				switch method {
				case "POST":
					// If no action suffix, assume create_machine or start_machine
					// If there's no distinct pattern, fallback to "update_machine"
					// But we already used create_machine for POST without ID.
					// We'll guess "update_machine" here.
					action = "update_machine"
				case "DELETE":
					action = "delete_machine"
				case "PATCH", "PUT":
					action = "update_machine"
				default:
					// fallback:
					action = "other_machine_action"
				}
			} else {
				// last segment might be an action: e.g. "/start"
				switch last {
				case "start":
					action = "start_machine"
				case "stop":
					action = "stop_machine"
				default:
					action = "other_machine_action"
				}
			}
			machineID = machineIDMatch[1]
			return action, machineID
		} else {
			// No machine_id in endpoint
			// Possibly /apps/{app}/machines (POST) -> create_machine
			if strings.HasPrefix(path, "/apps/") && strings.Contains(path, "/machines") {
				if method == "POST" {
					action = "create_machine"
					return action, ""
				}
				if method == "DELETE" {
					action = "delete_machines"
					return action, ""
				}
				// fallback
				action = "other_action"
				return action, ""
			}
			// fallback for unknown actions:
			action = "other_action"
			return action, ""
		}
	}
}

func (f *FlyIOAdapter) isRateLimited(action string, machineID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := action
	if machineID != "" {
		key = action + ":" + machineID
	} else {
		key = action + ":global"
	}

	limit := f.getRateLimitForAction(action)

	now := time.Now().Unix()
	windowStart := now - 1 // 1 second window
	timestamps := f.requestHistory[key]
	var newTimestamps []int64
	for _, ts := range timestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	f.requestHistory[key] = newTimestamps

	// Enforce rate limit: if length >= limit, blocked
	return len(newTimestamps) >= limit
}

func (f *FlyIOAdapter) recordRequest(action, machineID string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := action
	if machineID != "" {
		key = action + ":" + machineID
	} else {
		key = action + ":global"
	}

	timestamps := f.requestHistory[key]
	timestamps = append(timestamps, time.Now().Unix())
	f.requestHistory[key] = timestamps
}

func (f *FlyIOAdapter) getRateLimitForAction(action string) int {
	// "get_machine" -> 5 req/s
	if action == "get_machine" {
		return FlyIOGetMachineRate
	}
	// Others -> 1 req/s
	return FlyIOOtherRate
}
