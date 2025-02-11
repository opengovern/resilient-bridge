package adapters

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
)

const (
	RenderServicesCreateUpdateMax  = 20
	RenderServicesCreateUpdateSecs = 3600 // 1 hour

	RenderServicesDeployMax  = 10
	RenderServicesDeploySecs = 60 // 10/min

	RenderDeployHooksMax  = 10
	RenderDeployHooksSecs = 60

	RenderJobsMax  = 2000
	RenderJobsSecs = 3600 // 2000/hour

	RenderOtherWriteMax  = 30
	RenderOtherWriteSecs = 60 // 30/min

	RenderGetMax  = 400
	RenderGetSecs = 60 // 400/min
)

type RenderAdapter struct {
	APIToken string

	mu sync.Mutex
	// Maps category -> slice of timestamps
	requestHistory map[string][]int64

	// Maps category -> (maxRequests, windowSecs)
	categories map[string]struct {
		maxReq     int
		windowSecs int64
	}
}

var (
	renderServicesCreateUpdatePattern = regexp.MustCompile(`^/v1/services(\?|$|/[^/]+/(resume|suspend)$)`)
	renderServicesDeployPattern       = regexp.MustCompile(`^/v1/services/[^/]+/deploy`)
	renderDeployHooksPattern          = regexp.MustCompile(`^/v1/services/[^/]+/deployhook`)
	renderJobsPattern                 = regexp.MustCompile(`^/v1/jobs`)
	renderCustomDomainPattern         = regexp.MustCompile(`^/v1/customdomain`)
)

func NewRenderAdapter(apiToken string) *RenderAdapter {
	return &RenderAdapter{
		APIToken:       apiToken,
		requestHistory: make(map[string][]int64),
		categories: make(map[string]struct {
			maxReq     int
			windowSecs int64
		}),
	}
}

func (r *RenderAdapter) SetRateLimitDefaultsForType(requestType string, maxRequests int, windowSecs int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if maxRequests == 0 || windowSecs == 0 {
		switch requestType {
		case "services_create_update":
			if maxRequests == 0 {
				maxRequests = RenderServicesCreateUpdateMax
			}
			if windowSecs == 0 {
				windowSecs = RenderServicesCreateUpdateSecs
			}
		case "services_deploy":
			if maxRequests == 0 {
				maxRequests = RenderServicesDeployMax
			}
			if windowSecs == 0 {
				windowSecs = RenderServicesDeploySecs
			}
		case "deploy_hooks":
			if maxRequests == 0 {
				maxRequests = RenderDeployHooksMax
			}
			if windowSecs == 0 {
				windowSecs = RenderDeployHooksSecs
			}
		case "jobs":
			if maxRequests == 0 {
				maxRequests = RenderJobsMax
			}
			if windowSecs == 0 {
				windowSecs = RenderJobsSecs
			}
		case "other_write":
			if maxRequests == 0 {
				maxRequests = RenderOtherWriteMax
			}
			if windowSecs == 0 {
				windowSecs = RenderOtherWriteSecs
			}
		case "get":
			if maxRequests == 0 {
				maxRequests = RenderGetMax
			}
			if windowSecs == 0 {
				windowSecs = RenderGetSecs
			}
		default:
			if maxRequests == 0 {
				maxRequests = RenderOtherWriteMax
			}
			if windowSecs == 0 {
				windowSecs = RenderOtherWriteSecs
			}
		}
	}
	r.categories[requestType] = struct {
		maxReq     int
		windowSecs int64
	}{maxRequests, windowSecs}
}

// IdentifyRequestType: Render does not mention GraphQL. Assume all are "rest".
func (r *RenderAdapter) IdentifyRequestType(req *resilientbridge.NormalizedRequest) string {
	return "rest"
}

func (r *RenderAdapter) ExecuteRequest(req *resilientbridge.NormalizedRequest) (*resilientbridge.NormalizedResponse, error) {
	category := r.classifyRequest(req)
	if r.isRateLimited(category) {
		return &resilientbridge.NormalizedResponse{
			StatusCode: 429,
			Headers:    map[string]string{},
			Data:       []byte(`{"error":"Render rate limit reached"}`),
		}, nil
	}

	client := &http.Client{}
	fullURL := "https://api.render.com" + req.Endpoint

	httpReq, err := http.NewRequest(req.Method, fullURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Authorization") == "" && r.APIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+r.APIToken)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	r.recordRequest(category)

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

func (r *RenderAdapter) ParseRateLimitInfo(resp *resilientbridge.NormalizedResponse) (*resilientbridge.NormalizedRateLimitInfo, error) {
	h := resp.Headers
	parseInt := func(key string) *int {
		if val, ok := h[strings.ToLower(key)]; ok {
			if i, err := strconv.Atoi(val); err == nil {
				return resilientbridge.IntPtr(i)
			}
		}
		return nil
	}

	parseReset := func(key string) *int64 {
		if val, ok := h[strings.ToLower(key)]; ok {
			if ts, err := strconv.ParseInt(val, 10, 64); err == nil {
				ms := ts * 1000
				return &ms
			}
		}
		return nil
	}

	info := &resilientbridge.NormalizedRateLimitInfo{
		MaxRequests:       parseInt("ratelimit-limit"),
		RemainingRequests: parseInt("ratelimit-remaining"),
		ResetRequestsAt:   parseReset("ratelimit-reset"),
	}
	return info, nil
}

func (r *RenderAdapter) IsRateLimitError(resp *resilientbridge.NormalizedResponse) bool {
	return resp.StatusCode == 429
}

func (r *RenderAdapter) classifyRequest(req *resilientbridge.NormalizedRequest) string {
	endpoint := req.Endpoint
	method := strings.ToUpper(req.Method)

	if method == "POST" && strings.HasPrefix(endpoint, "/v1/services") &&
		!renderServicesDeployPattern.MatchString(endpoint) &&
		!renderServicesCreateUpdatePattern.MatchString(endpoint) {
		// If not matched by pattern, fallback
	}

	// The logic from original code is retained as is.
	if method == "POST" && strings.HasPrefix(endpoint, "/v1/services") {
		if strings.HasPrefix(endpoint, "/v1/services?") || endpoint == "/v1/services" || renderServicesCreateUpdatePattern.MatchString(endpoint) {
			return "services_create_update"
		}
		if renderServicesDeployPattern.MatchString(endpoint) {
			return "services_deploy"
		}
	}

	if method == "PATCH" && strings.HasPrefix(endpoint, "/v1/services") {
		return "services_create_update"
	}

	if method == "POST" && renderServicesDeployPattern.MatchString(endpoint) {
		return "services_deploy"
	}

	if method == "POST" && renderDeployHooksPattern.MatchString(endpoint) {
		return "deploy_hooks"
	}

	if method == "POST" && renderCustomDomainPattern.MatchString(endpoint) {
		return "other_write"
	}

	if method == "POST" && renderJobsPattern.MatchString(endpoint) {
		return "jobs"
	}

	if (method == "POST" || method == "PATCH" || method == "DELETE") && !strings.HasPrefix(endpoint, "/v1/services") && !renderJobsPattern.MatchString(endpoint) && !renderCustomDomainPattern.MatchString(endpoint) {
		return "other_write"
	}

	if method == "GET" {
		return "get"
	}

	return "other_write"
}

func (r *RenderAdapter) isRateLimited(category string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	cat, ok := r.categories[category]
	if !ok {
		// default to other_write if unknown
		cat = struct {
			maxReq     int
			windowSecs int64
		}{RenderOtherWriteMax, RenderOtherWriteSecs}
		r.categories[category] = cat
	}

	now := time.Now().Unix()
	windowStart := now - cat.windowSecs
	timestamps := r.requestHistory[category]
	var newTimestamps []int64
	for _, ts := range timestamps {
		if ts >= windowStart {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	r.requestHistory[category] = newTimestamps

	return len(newTimestamps) >= cat.maxReq
}

func (r *RenderAdapter) recordRequest(category string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	timestamps := r.requestHistory[category]
	timestamps = append(timestamps, time.Now().Unix())
	r.requestHistory[category] = timestamps
}
