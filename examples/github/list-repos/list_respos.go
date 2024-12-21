package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// MAX_REPO limits how many repositories we retrieve in "list" mode.
const MAX_REPO = 250

// ---- STRUCT DEFINITIONS ----------------------------------------------------

type License struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	SPDXID string `json:"spdx_id"`
	URL    string `json:"url"`
	NodeID string `json:"node_id"`
}

type Permissions struct {
	Admin    bool `json:"admin"`
	Maintain bool `json:"maintain"`
	Push     bool `json:"push"`
	Triage   bool `json:"triage"`
	Pull     bool `json:"pull"`
}

type StatusObj struct {
	Status string `json:"status"`
}

type RepoDetail struct {
	ID                        int                    `json:"id"`
	NodeID                    string                 `json:"node_id"`
	Name                      string                 `json:"name"`
	FullName                  string                 `json:"full_name"`
	Private                   bool                   `json:"private"`
	Owner                     *Owner                 `json:"owner"`
	HTMLURL                   string                 `json:"html_url"`
	Description               *string                `json:"description"`
	Fork                      bool                   `json:"fork"`
	CreatedAt                 string                 `json:"created_at"`
	UpdatedAt                 string                 `json:"updated_at"`
	PushedAt                  string                 `json:"pushed_at"`
	GitURL                    string                 `json:"git_url"`
	SSHURL                    string                 `json:"ssh_url"`
	CloneURL                  string                 `json:"clone_url"`
	SVNURL                    string                 `json:"svn_url"`
	Homepage                  *string                `json:"homepage"`
	Size                      int                    `json:"size"`
	StargazersCount           int                    `json:"stargazers_count"`
	WatchersCount             int                    `json:"watchers_count"`
	Languages                 *string                `json:"languages"`
	HasIssues                 bool                   `json:"has_issues"`
	HasProjects               bool                   `json:"has_projects"`
	HasDownloads              bool                   `json:"has_downloads"`
	HasWiki                   bool                   `json:"has_wiki"`
	HasPages                  bool                   `json:"has_pages"`
	HasDiscussions            bool                   `json:"has_discussions"`
	ForksCount                int                    `json:"forks_count"`
	MirrorURL                 *string                `json:"mirror_url"`
	Archived                  bool                   `json:"archived"`
	Disabled                  bool                   `json:"disabled"`
	OpenIssuesCount           int                    `json:"open_issues_count"`
	License                   *License               `json:"license"`
	AllowForking              bool                   `json:"allow_forking"`
	IsTemplate                bool                   `json:"is_template"`
	WebCommitSignoffRequired  bool                   `json:"web_commit_signoff_required"`
	Topics                    []string               `json:"topics"`
	Visibility                string                 `json:"visibility"`
	DefaultBranch             string                 `json:"default_branch"`
	Permissions               *Permissions           `json:"permissions"`
	AllowSquashMerge          bool                   `json:"allow_squash_merge"`
	AllowMergeCommit          bool                   `json:"allow_merge_commit"`
	AllowRebaseMerge          bool                   `json:"allow_rebase_merge"`
	AllowAutoMerge            bool                   `json:"allow_auto_merge"`
	DeleteBranchOnMerge       bool                   `json:"delete_branch_on_merge"`
	AllowUpdateBranch         bool                   `json:"allow_update_branch"`
	UseSquashPRTitleAsDefault bool                   `json:"use_squash_pr_title_as_default"`
	SquashMergeCommitMessage  string                 `json:"squash_merge_commit_message"`
	SquashMergeCommitTitle    string                 `json:"squash_merge_commit_title"`
	MergeCommitMessage        string                 `json:"merge_commit_message"`
	MergeCommitTitle          string                 `json:"merge_commit_title"`
	CustomProperties          map[string]interface{} `json:"custom_properties"`
	Organization              *Organization          `json:"organization"`
	Parent                    *RepoDetail            `json:"parent"`
	Source                    *RepoDetail            `json:"source"`
	NetworkCount              int                    `json:"network_count"`
	SubscribersCount          int                    `json:"subscribers_count"`
	BlankIssuesEnabled        bool                   `json:"blank_issues_enabled"`
	Locked                    bool                   `json:"locked"`

	SecurityAndAnalysis *struct {
		SecretScanning                    *StatusObj `json:"secret_scanning"`
		SecretScanningPushProtection      *StatusObj `json:"secret_scanning_push_protection"`
		DependabotSecurityUpdates         *StatusObj `json:"dependabot_security_updates"`
		SecretScanningNonProviderPatterns *StatusObj `json:"secret_scanning_non_provider_patterns"`
		SecretScanningValidityChecks      *StatusObj `json:"secret_scanning_validity_checks"`
	} `json:"security_and_analysis"`
}

type RepositorySettings struct {
	HasDiscussionsEnabled     bool                   `json:"has_discussions_enabled"`
	HasIssuesEnabled          bool                   `json:"has_issues_enabled"`
	HasProjectsEnabled        bool                   `json:"has_projects_enabled"`
	HasWikiEnabled            bool                   `json:"has_wiki_enabled"`
	MergeCommitAllowed        bool                   `json:"merge_commit_allowed"`
	MergeCommitMessage        string                 `json:"merge_commit_message"`
	MergeCommitTitle          string                 `json:"merge_commit_title"`
	SquashMergeAllowed        bool                   `json:"squash_merge_allowed"`
	SquashMergeCommitMessage  string                 `json:"squash_merge_commit_message"`
	SquashMergeCommitTitle    string                 `json:"squash_merge_commit_title"`
	HasDownloads              bool                   `json:"has_downloads"`
	HasPages                  bool                   `json:"has_pages"`
	WebCommitSignoffRequired  bool                   `json:"web_commit_signoff_required"`
	MirrorURL                 *string                `json:"mirror_url"`
	AllowAutoMerge            bool                   `json:"allow_auto_merge"`
	DeleteBranchOnMerge       bool                   `json:"delete_branch_on_merge"`
	AllowUpdateBranch         bool                   `json:"allow_update_branch"`
	UseSquashPRTitleAsDefault bool                   `json:"use_squash_pr_title_as_default"`
	CustomProperties          map[string]interface{} `json:"custom_properties"`
	ForkingAllowed            bool                   `json:"forking_allowed"`
	IsTemplate                bool                   `json:"is_template"`
	AllowRebaseMerge          bool                   `json:"allow_rebase_merge"`
	Archived                  bool                   `json:"archived"`
	Disabled                  bool                   `json:"disabled"`
	Locked                    bool                   `json:"locked"`
}

type SecuritySettings struct {
	VulnerabilityAlertsEnabled               bool `json:"vulnerability_alerts_enabled"`
	SecretScanningEnabled                    bool `json:"secret_scanning_enabled"`
	SecretScanningPushProtectionEnabled      bool `json:"secret_scanning_push_protection_enabled"`
	DependabotSecurityUpdatesEnabled         bool `json:"dependabot_security_updates_enabled"`
	SecretScanningNonProviderPatternsEnabled bool `json:"secret_scanning_non_provider_patterns_enabled"`
	SecretScanningValidityChecksEnabled      bool `json:"secret_scanning_validity_checks_enabled"`
}

type RepoURLs struct {
	GitURL   string `json:"git_url"`
	SSHURL   string `json:"ssh_url"`
	CloneURL string `json:"clone_url"`
	SVNURL   string `json:"svn_url"`
	HTMLURL  string `json:"html_url"`
}

type Owner struct {
	Login   string `json:"login"`
	ID      int    `json:"id"`
	NodeID  string `json:"node_id"`
	HTMLURL string `json:"html_url"`
	Type    string `json:"type"`
}

type Organization struct {
	Login        string `json:"login"`
	ID           int    `json:"id"`
	NodeID       string `json:"node_id"`
	HTMLURL      string `json:"html_url"`
	Type         string `json:"type"`
	UserViewType string `json:"user_view_type"`
	SiteAdmin    bool   `json:"site_admin"`
}

type Metrics struct {
	Stargazers   int `json:"stargazers"`
	Forks        int `json:"forks"`
	Subscribers  int `json:"subscribers"`
	Size         int `json:"size"`
	Tags         int `json:"tags"`
	Commits      int `json:"commits"`
	Issues       int `json:"issues"`
	OpenIssues   int `json:"open_issues"`
	Branches     int `json:"branches"`
	PullRequests int `json:"pull_requests"`
	Releases     int `json:"releases"`
}

type FinalRepoDetail struct {
	GitHubRepoID            int                `json:"id"`
	NodeID                  string             `json:"node_id"`
	Name                    string             `json:"name"`
	NameWithOwner           string             `json:"name_with_owner"`
	Description             *string            `json:"description"`
	CreatedAt               string             `json:"created_at"`
	UpdatedAt               string             `json:"updated_at"`
	PushedAt                string             `json:"pushed_at"`
	IsActive                bool               `json:"is_active"`
	IsEmpty                 bool               `json:"is_empty"`
	IsFork                  bool               `json:"is_fork"`
	IsSecurityPolicyEnabled bool               `json:"is_security_policy_enabled"`
	Owner                   *Owner             `json:"owner"`
	HomepageURL             *string            `json:"homepage_url"`
	LicenseInfo             json.RawMessage    `json:"license_info"`
	Topics                  []string           `json:"topics"`
	Visibility              string             `json:"visibility"`
	DefaultBranchRef        json.RawMessage    `json:"default_branch_ref"`
	Permissions             *Permissions       `json:"permissions"`
	Organization            *Organization      `json:"organization"`
	Parent                  *FinalRepoDetail   `json:"parent"`
	Source                  *FinalRepoDetail   `json:"source"`
	Language                map[string]int     `json:"language"`
	RepositorySettings      RepositorySettings `json:"repo_settings"`
	SecuritySettings        SecuritySettings   `json:"security_settings"`
	RepoURLs                RepoURLs           `json:"repo_urls"`
	Metrics                 Metrics            `json:"metrics"`
}

// ---------------------------------------------------------------------------
// 1. GetRepoList: Lists repositories for an organization up to MAX_REPO,
//                 returning them as a slice of FinalRepoDetail (partial info).
// ---------------------------------------------------------------------------

func GetRepoList(orgName string) ([]FinalRepoDetail, error) {
	if orgName == "" {
		return nil, fmt.Errorf("orgName must be provided")
	}

	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Println("GITHUB_API_TOKEN not set; you may only be able to access public repos")
	}

	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	var allRepos []FinalRepoDetail
	page := 1
	for len(allRepos) < MAX_REPO {
		remaining := MAX_REPO - len(allRepos)
		perPage := 100
		if remaining < perPage {
			perPage = remaining
		}

		endpoint := fmt.Sprintf("/orgs/%s/repos?per_page=%d&page=%d", orgName, perPage, page)
		req := &resilientbridge.NormalizedRequest{
			Method:   "GET",
			Endpoint: endpoint,
			Headers:  map[string]string{"Accept": "application/vnd.github+json"},
		}

		resp, err := sdk.Request("github", req)
		if err != nil {
			return nil, fmt.Errorf("error fetching org repos: %w", err)
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
		}

		var partialList []RepoDetail
		if err := json.Unmarshal(resp.Data, &partialList); err != nil {
			return nil, fmt.Errorf("error decoding repos list: %w", err)
		}
		if len(partialList) == 0 {
			break
		}

		// Transform each partial RepoDetail to FinalRepoDetail (most fields empty).
		for _, repo := range partialList {
			finalRepo := util_transformToFinalRepoDetail(&repo)
			allRepos = append(allRepos, *finalRepo)
		}

		if len(allRepos) >= MAX_REPO {
			break
		}
		page++
	}

	return allRepos, nil
}

// ---------------------------------------------------------------------------
// 2. GetRepository: Fetches full detail for a single repo, enriches it,
//                   and returns JSON string.
// ---------------------------------------------------------------------------

func GetRepository(orgName, repoName string) (string, error) {
	if orgName == "" || repoName == "" {
		return "", fmt.Errorf("both orgName and repoName must be provided")
	}

	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Println("GITHUB_API_TOKEN not set; you may only be able to access public repos")
	}

	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	repoDetail, err := util_fetchRepoDetails(sdk, orgName, repoName)
	if err != nil {
		return "", fmt.Errorf("error fetching repository details: %w", err)
	}

	finalDetail := util_transformToFinalRepoDetail(repoDetail)

	// Fetch languages
	langs, err := util_fetchLanguages(sdk, orgName, repoName)
	if err == nil && len(langs) > 0 {
		finalDetail.Language = langs
	} else {
		finalDetail.Language = nil
	}

	// Enrich metrics
	if err := util_enrichRepoMetrics(sdk, orgName, repoName, finalDetail); err != nil {
		log.Printf("Error enriching repo metrics for %s/%s: %v", orgName, repoName, err)
	}

	data, err := json.MarshalIndent(finalDetail, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshalling repo detail: %w", err)
	}

	return string(data), nil
}

// ---------------------------------------------------------------------------
// 3. main(): If only an organization is provided, fetch full list via
//            GetRepoList(), then call GetRepository() for each to print JSON.
//            If org/repo is provided, just call GetRepository() and print JSON.
// ---------------------------------------------------------------------------

func main() {
	repoFlag := flag.String("repo", "", "Repository or organization in format https://github.com/<org> or https://github.com/<org>/<repo>")
	flag.Parse()

	if *repoFlag == "" {
		log.Fatal("You must provide a -repo parameter (https://github.com/<org> or https://github.com/<org>/<repo>)")
	}

	owner, repoName, err := util_parseScopeURL(*repoFlag)
	if err != nil {
		log.Fatalf("Error parsing scope URL: %v", err)
	}

	// If only organization is provided, fetch the list, then get each one.
	if repoName == "" {
		allRepos, err := GetRepoList(owner)
		if err != nil {
			log.Fatalf("Error fetching organization repositories: %v", err)
		}

		for _, repo := range allRepos {
			// The "Name" field in FinalRepoDetail is the actual repo name
			jsonData, err := GetRepository(owner, repo.Name)
			if err != nil {
				log.Printf("Error fetching details for %s/%s: %v", owner, repo.Name, err)
				continue
			}
			fmt.Println(jsonData)
		}
	} else {
		// If owner + repo is provided, just fetch that single repo.
		jsonData, err := GetRepository(owner, repoName)
		if err != nil {
			log.Fatalf("Error fetching repository details: %v", err)
		}
		fmt.Println(jsonData)
	}
}

// ---- HELPER FUNCTIONS (all now prefixed with util_) -----------------------

// Creates a consistent FinalRepoDetail from a RepoDetail, ensuring missing
// fields become nil or an empty fallback (like an empty slice for `topics`).
func util_transformToFinalRepoDetail(detail *RepoDetail) *FinalRepoDetail {
	var parent *FinalRepoDetail
	if detail.Parent != nil {
		parent = util_transformToFinalRepoDetail(detail.Parent)
	}
	var source *FinalRepoDetail
	if detail.Source != nil {
		source = util_transformToFinalRepoDetail(detail.Source)
	}

	// LicenseInfo
	var licenseJSON json.RawMessage
	if detail.License != nil {
		lj, _ := json.Marshal(detail.License)
		licenseJSON = lj
	} else {
		licenseJSON = nil
	}

	topics := detail.Topics
	if topics == nil {
		topics = []string{}
	}

	dbObj := map[string]string{"name": detail.DefaultBranch}
	dbBytes, _ := json.Marshal(dbObj)

	isActive := !(detail.Archived || detail.Disabled)
	isEmpty := (detail.Size == 0)

	var customProps map[string]interface{}
	if detail.CustomProperties == nil {
		customProps = map[string]interface{}{}
	} else {
		customProps = detail.CustomProperties
	}

	finalDetail := &FinalRepoDetail{
		GitHubRepoID:            detail.ID,
		NodeID:                  detail.NodeID,
		Name:                    detail.Name,
		NameWithOwner:           detail.FullName,
		Description:             detail.Description,
		CreatedAt:               detail.CreatedAt,
		UpdatedAt:               detail.UpdatedAt,
		PushedAt:                detail.PushedAt,
		IsActive:                isActive,
		IsEmpty:                 isEmpty,
		IsFork:                  detail.Fork,
		IsSecurityPolicyEnabled: false,
		Owner:                   detail.Owner,
		HomepageURL:             detail.Homepage,
		LicenseInfo:             licenseJSON,
		Topics:                  topics,
		Visibility:              detail.Visibility,
		DefaultBranchRef:        dbBytes,
		Permissions:             detail.Permissions,
		Organization:            detail.Organization,
		Parent:                  parent,
		Source:                  source,
		Language:                nil, // filled later
		RepositorySettings: RepositorySettings{
			HasDiscussionsEnabled:     detail.HasDiscussions,
			HasIssuesEnabled:          detail.HasIssues,
			HasProjectsEnabled:        detail.HasProjects,
			HasWikiEnabled:            detail.HasWiki,
			MergeCommitAllowed:        detail.AllowMergeCommit,
			MergeCommitMessage:        detail.MergeCommitMessage,
			MergeCommitTitle:          detail.MergeCommitTitle,
			SquashMergeAllowed:        detail.AllowSquashMerge,
			SquashMergeCommitMessage:  detail.SquashMergeCommitMessage,
			SquashMergeCommitTitle:    detail.SquashMergeCommitTitle,
			HasDownloads:              detail.HasDownloads,
			HasPages:                  detail.HasPages,
			WebCommitSignoffRequired:  detail.WebCommitSignoffRequired,
			MirrorURL:                 detail.MirrorURL,
			AllowAutoMerge:            detail.AllowAutoMerge,
			DeleteBranchOnMerge:       detail.DeleteBranchOnMerge,
			AllowUpdateBranch:         detail.AllowUpdateBranch,
			UseSquashPRTitleAsDefault: detail.UseSquashPRTitleAsDefault,
			CustomProperties:          customProps,
			ForkingAllowed:            detail.AllowForking,
			IsTemplate:                detail.IsTemplate,
			AllowRebaseMerge:          detail.AllowRebaseMerge,
			Archived:                  detail.Archived,
			Disabled:                  detail.Disabled,
			Locked:                    detail.Locked,
		},
		SecuritySettings: SecuritySettings{
			VulnerabilityAlertsEnabled:               false,
			SecretScanningEnabled:                    false,
			SecretScanningPushProtectionEnabled:      false,
			DependabotSecurityUpdatesEnabled:         false,
			SecretScanningNonProviderPatternsEnabled: false,
			SecretScanningValidityChecksEnabled:      false,
		},
		RepoURLs: RepoURLs{
			GitURL:   detail.GitURL,
			SSHURL:   detail.SSHURL,
			CloneURL: detail.CloneURL,
			SVNURL:   detail.SVNURL,
			HTMLURL:  detail.HTMLURL,
		},
		Metrics: Metrics{
			Stargazers:  detail.StargazersCount,
			Forks:       detail.ForksCount,
			Subscribers: detail.SubscribersCount,
			Size:        detail.Size,
			OpenIssues:  detail.OpenIssuesCount,
		},
	}

	return finalDetail
}

func util_fetchRepoDetails(sdk *resilientbridge.ResilientBridge, owner, repo string) (*RepoDetail, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/repos/%s/%s", owner, repo),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}
	resp, err := sdk.Request("github", req)
	if err != nil {
		return nil, fmt.Errorf("error fetching repo details: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var detail RepoDetail
	if err := json.Unmarshal(resp.Data, &detail); err != nil {
		return nil, fmt.Errorf("error decoding repo details: %w", err)
	}
	return &detail, nil
}

func util_fetchLanguages(sdk *resilientbridge.ResilientBridge, owner, repo string) (map[string]int, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/repos/%s/%s/languages", owner, repo),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}
	resp, err := sdk.Request("github", req)
	if err != nil {
		return nil, fmt.Errorf("error fetching languages: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var langs map[string]int
	if err := json.Unmarshal(resp.Data, &langs); err != nil {
		return nil, fmt.Errorf("error decoding languages: %w", err)
	}
	return langs, nil
}

func util_enrichRepoMetrics(sdk *resilientbridge.ResilientBridge, owner, repoName string, finalDetail *FinalRepoDetail) error {
	var dbObj map[string]string
	if finalDetail.DefaultBranchRef != nil {
		if err := json.Unmarshal(finalDetail.DefaultBranchRef, &dbObj); err != nil {
			return err
		}
	}
	defaultBranch := dbObj["name"]
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	commitsCount, err := util_countCommits(sdk, owner, repoName, defaultBranch)
	if err != nil {
		return fmt.Errorf("counting commits: %w", err)
	}
	finalDetail.Metrics.Commits = commitsCount

	issuesCount, err := util_countIssues(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting issues: %w", err)
	}
	finalDetail.Metrics.Issues = issuesCount

	branchesCount, err := util_countBranches(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting branches: %w", err)
	}
	finalDetail.Metrics.Branches = branchesCount

	prCount, err := util_countPullRequests(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting PRs: %w", err)
	}
	finalDetail.Metrics.PullRequests = prCount

	releasesCount, err := util_countReleases(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting releases: %w", err)
	}
	finalDetail.Metrics.Releases = releasesCount

	tagsCount, err := util_countTags(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting tags: %w", err)
	}
	finalDetail.Metrics.Tags = tagsCount

	return nil
}

func util_countTags(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/tags?per_page=1", owner, repoName)
	return util_countItemsFromEndpoint(sdk, endpoint)
}

func util_countCommits(sdk *resilientbridge.ResilientBridge, owner, repoName, defaultBranch string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits?sha=%s&per_page=1", owner, repoName, defaultBranch)
	return util_countItemsFromEndpoint(sdk, endpoint)
}

func util_countIssues(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/issues?state=all&per_page=1", owner, repoName)
	return util_countItemsFromEndpoint(sdk, endpoint)
}

func util_countBranches(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/branches?per_page=1", owner, repoName)
	return util_countItemsFromEndpoint(sdk, endpoint)
}

func util_countPullRequests(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=all&per_page=1", owner, repoName)
	return util_countItemsFromEndpoint(sdk, endpoint)
}

func util_countReleases(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/releases?per_page=1", owner, repoName)
	return util_countItemsFromEndpoint(sdk, endpoint)
}

func util_countItemsFromEndpoint(sdk *resilientbridge.ResilientBridge, endpoint string) (int, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}
	resp, err := sdk.Request("github", req)
	if err != nil {
		return 0, fmt.Errorf("error fetching data: %w", err)
	}

	// Some repos return 409 for certain endpoints (empty or not applicable).
	if resp.StatusCode == 409 {
		return 0, nil
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var linkHeader string
	for k, v := range resp.Headers {
		if strings.ToLower(k) == "link" {
			linkHeader = v
			break
		}
	}

	// If there's no Link header, the count is the length of the JSON array (or 0 if empty).
	if linkHeader == "" {
		if len(resp.Data) > 2 {
			var items []interface{}
			if err := json.Unmarshal(resp.Data, &items); err != nil {
				// If we can't unmarshal, assume there's at least one item.
				return 1, nil
			}
			return len(items), nil
		}
		return 0, nil
	}

	lastPage, err := util_parseLastPage(linkHeader)
	if err != nil {
		return 0, fmt.Errorf("could not parse last page: %w", err)
	}
	return lastPage, nil
}

func util_parseLastPage(linkHeader string) (int, error) {
	re := regexp.MustCompile(`page=(\d+)>; rel="last"`)
	matches := re.FindStringSubmatch(linkHeader)
	if len(matches) < 2 {
		return 1, nil
	}
	var lastPage int
	_, err := fmt.Sscanf(matches[1], "%d", &lastPage)
	if err != nil {
		return 0, err
	}
	return lastPage, nil
}

func util_parseScopeURL(repoURL string) (owner, repo string, err error) {
	if !strings.HasPrefix(repoURL, "https://github.com/") {
		return "", "", fmt.Errorf("URL must start with https://github.com/")
	}
	parts := strings.Split(strings.TrimPrefix(repoURL, "https://github.com/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", fmt.Errorf("invalid URL format")
	}
	owner = parts[0]
	if len(parts) > 1 {
		repo = parts[1]
	}
	return owner, repo, nil
}
