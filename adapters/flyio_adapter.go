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
)

// We'll use regex to identify endpoints and actions.
var machineIDPattern = regexp.MustCompile(`^/apps/[^/]+/machines/([^/]+)(/.*)?$`)

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
	// Fly.io has fixed rules, ignoring overrides for now.
}

func (f *FlyIOAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	// Fly.io adapter currently does not differentiate between rest/graphql or other types.
	// Assume all are "rest" since no GraphQL context is mentioned.
	return "rest"
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
	baseURL := "https://api.machines.dev"
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
	// No explicit rate limit headers to parse.
	return nil, nil
}

func (f *FlyIOAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (f *FlyIOAdapter) classifyRequest(req *resilientbridge.NormalizedRequest) (action string, machineID string) {
	path := req.Endpoint
	method := strings.ToUpper(req.Method)

	if method == "GET" {
		if machineIDMatch := machineIDPattern.FindStringSubmatch(path); machineIDMatch != nil {
			action = "get_machine"
			machineID = machineIDMatch[1]
			return action, machineID
		} else {
			action = "list_machines"
			return action, ""
		}
	} else {
		if machineIDMatch := machineIDPattern.FindStringSubmatch(path); machineIDMatch != nil {
			parts := strings.Split(path, "/")
			last := parts[len(parts)-1]
			if last == machineIDMatch[1] {
				switch method {
				case "POST":
					action = "update_machine"
				case "DELETE":
					action = "delete_machine"
				case "PATCH", "PUT":
					action = "update_machine"
				default:
					action = "other_machine_action"
				}
			} else {
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
			if strings.HasPrefix(path, "/apps/") && strings.Contains(path, "/machines") {
				if method == "POST" {
					action = "create_machine"
					return action, ""
				}
				if method == "DELETE" {
					action = "delete_machines"
					return action, ""
				}
				action = "other_action"
				return action, ""
			}
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

	if f.requestHistory == nil {
		f.requestHistory = make(map[string][]int64)
	}

	limit := f.getRateLimitForAction(action)
	now := time.Now().Unix()
	windowStart := now - 1 // 1 second window for simplicity
	timestamps := f.requestHistory[key]
	var newTimestamps []int64
	for _, ts := range timestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	f.requestHistory[key] = newTimestamps

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
	if action == "get_machine" {
		return FlyIOGetMachineRate
	}
	return FlyIOOtherRate
}
