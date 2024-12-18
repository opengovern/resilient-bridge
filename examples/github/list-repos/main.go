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

type UserOrOrg struct {
	Login        string `json:"login"`
	ID           int    `json:"id"`
	NodeID       string `json:"node_id"`
	HTMLURL      string `json:"html_url"`
	Type         string `json:"type"`
	UserViewType string `json:"user_view_type"`
	SiteAdmin    bool   `json:"site_admin"`
}

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
	Owner                     *UserOrOrg             `json:"owner"`
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
	Language                  *string                `json:"language"`
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
	Organization              *UserOrOrg             `json:"organization"`
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
	HasDiscussionsEnabled bool `json:"has_discussions_enabled"`
	HasIssuesEnabled      bool `json:"has_issues_enabled"`
	HasProjectsEnabled    bool `json:"has_projects_enabled"`
	HasWikiEnabled        bool `json:"has_wiki_enabled"`

	MergeCommitAllowed bool   `json:"merge_commit_allowed"`
	MergeCommitMessage string `json:"merge_commit_message"`
	MergeCommitTitle   string `json:"merge_commit_title"`

	SquashMergeAllowed       bool   `json:"squash_merge_allowed"`
	SquashMergeCommitMessage string `json:"squash_merge_commit_message"`
	SquashMergeCommitTitle   string `json:"squash_merge_commit_title"`

	HasDownloads             bool `json:"has_downloads"`
	HasPages                 bool `json:"has_pages"`
	WebCommitSignoffRequired bool `json:"web_commit_signoff_required"`

	MirrorURL                 *string                `json:"mirror_url"`
	AllowAutoMerge            bool                   `json:"allow_auto_merge"`
	DeleteBranchOnMerge       bool                   `json:"delete_branch_on_merge"`
	AllowUpdateBranch         bool                   `json:"allow_update_branch"`
	UseSquashPRTitleAsDefault bool                   `json:"use_squash_pr_title_as_default"`
	CustomProperties          map[string]interface{} `json:"custom_properties"`
	ForkingAllowed            bool                   `json:"forking_allowed"`
	IsTemplate                bool                   `json:"is_template"`
	AllowRebaseMerge          bool                   `json:"allow_rebase_merge"`

	// Move is_archived and is_disabled here:
	IsArchived bool `json:"is_archived"`
	IsDisabled bool `json:"is_disabled"`
}

type SecuritySettings struct {
	VulnerabilityAlertsEnabled          bool `json:"vulnerability_alerts_enabled"`
	SecretScanningEnabled               bool `json:"secret_scanning_enabled"`
	SecretScanningPushProtectionEnabled bool `json:"secret_scanning_push_protection_enabled"`
	DependabotSecurityUpdatesEnabled    bool `json:"dependabot_security_updates_enabled"`
	SecretScanningNonProviderPatterns   bool `json:"secret_scanning_non_provider_patterns_enabled"`
	SecretScanningValidityChecksEnabled bool `json:"secret_scanning_validity_checks_enabled"`
}

type RepoURLs struct {
	GitURL   string `json:"git_url"`
	SSHURL   string `json:"ssh_url"`
	CloneURL string `json:"clone_url"`
	SVNURL   string `json:"svn_url"`
	HTMLURL  string `json:"html_url"`
}

type RepoMetrics struct {
	StargazersCount    int `json:"stargazer_count"`
	WatchersTotalCount int `json:"watchers_total_count"`
	ForkCount          int `json:"fork_count"`
	OpenIssuesCount    int `json:"open_issues_total_count"`
	NetworkCount       int `json:"network_count"`
	SubscribersCount   int `json:"subscribers_count"`
	Size               int `json:"size"`
	TotalCommits       int `json:"total_commits"`
	TotalIssues        int `json:"total_issues"`
	TotalBranches      int `json:"total_branches"`
	TotalPullRequests  int `json:"total_pull_requests"`
	TotalReleases      int `json:"total_releases"`
}

type FinalRepoDetail struct {
	GitHubRepoID  int    `json:"id"`
	NodeID        string `json:"node_id"`
	Name          string `json:"name"`
	NameWithOwner string `json:"name_with_owner"`

	// Add is_active at top level: repo is active if not archived and not disabled.
	IsActive bool `json:"is_active"`

	IsPrivate   bool       `json:"is_private"`
	Owner       *UserOrOrg `json:"owner"`
	Description *string    `json:"description"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
	PushedAt    string     `json:"pushed_at"`
	HomepageURL *string    `json:"homepage_url"`

	LicenseInfo json.RawMessage `json:"license_info"`

	Topics     []string `json:"topics"`
	Visibility string   `json:"visibility"`

	DefaultBranchRef json.RawMessage `json:"default_branch_ref"`

	Permissions  *Permissions     `json:"permissions"`
	Organization *UserOrOrg       `json:"organization"`
	Parent       *FinalRepoDetail `json:"parent"`
	Source       *FinalRepoDetail `json:"source"`
	Language     *string          `json:"language"`

	RepositorySettings RepositorySettings `json:"repo_settings"`
	SecuritySettings   SecuritySettings   `json:"security_settings"`

	RepoURLs    RepoURLs    `json:"repo_urls"`
	RepoMetrics RepoMetrics `json:"repo_metrics"`

	IsEmpty                       bool `json:"is_empty"`
	IsFork                        bool `json:"is_fork"`
	IsInOrganization              bool `json:"is_in_organization"`
	Locked                        bool `json:"locked"`
	IsMirror                      bool `json:"is_mirror"`
	IsSecurityPolicyEnabled       bool `json:"is_security_policy_enabled"`
	IsTemplate                    bool `json:"is_template"`
	IsUserConfigurationRepository bool `json:"is_user_configuration_repository"`
}

type MinimalRepoInfo struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

func main() {
	repoFlag := flag.String("repo", "", "Repository or organization in format https://github.com/<org> or https://github.com/<org>/<repo>")
	maxResultsFlag := flag.Int("max-results", 100, "Maximum number of repositories to fetch (default 100) for org listing")
	flag.Parse()

	if *repoFlag == "" {
		log.Fatal("You must provide a -repo parameter")
	}

	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Println("GITHUB_API_TOKEN not set; you may only be able to access public repos")
	}

	owner, repoName, err := parseScopeURL(*repoFlag)
	if err != nil {
		log.Fatalf("Error parsing scope URL: %v", err)
	}

	maxResults := *maxResultsFlag
	if maxResults <= 0 {
		maxResults = 100
	}

	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	if repoName == "" {
		allRepos, err := fetchOrgRepos(sdk, owner, maxResults)
		if err != nil {
			log.Fatalf("Error fetching organization repositories: %v", err)
		}

		for _, r := range allRepos {
			repoDetail, err := fetchRepoDetails(sdk, r.Owner.Login, r.Name)
			if err != nil {
				log.Printf("Error fetching details for %s/%s: %v", r.Owner.Login, r.Name, err)
				continue
			}

			finalDetail := transformToFinalRepoDetail(repoDetail)

			err = enrichRepoMetrics(sdk, r.Owner.Login, r.Name, finalDetail)
			if err != nil {
				log.Printf("Error enriching repo metrics for %s/%s: %v", r.Owner.Login, r.Name, err)
			}

			data, err := json.MarshalIndent(finalDetail, "", "  ")
			if err != nil {
				log.Printf("Error marshalling repo detail for %s/%s: %v", r.Owner.Login, r.Name, err)
				continue
			}
			fmt.Println(string(data))
		}
	} else {
		repoDetail, err := fetchRepoDetails(sdk, owner, repoName)
		if err != nil {
			log.Fatalf("Error fetching repository details: %v", err)
		}

		finalDetail := transformToFinalRepoDetail(repoDetail)

		err = enrichRepoMetrics(sdk, owner, repoName, finalDetail)
		if err != nil {
			log.Printf("Error enriching repo metrics for %s/%s: %v", owner, repoName, err)
		}

		data, err := json.MarshalIndent(finalDetail, "", "  ")
		if err != nil {
			log.Fatalf("Error marshalling repo detail: %v", err)
		}
		fmt.Println(string(data))
	}
}

func enrichRepoMetrics(sdk *resilientbridge.ResilientBridge, owner, repoName string, finalDetail *FinalRepoDetail) error {
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

	commitsCount, err := countCommits(sdk, owner, repoName, defaultBranch)
	if err != nil {
		return fmt.Errorf("counting commits: %w", err)
	}
	finalDetail.RepoMetrics.TotalCommits = commitsCount

	issuesCount, err := countIssues(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting issues: %w", err)
	}
	finalDetail.RepoMetrics.TotalIssues = issuesCount

	branchesCount, err := countBranches(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting branches: %w", err)
	}
	finalDetail.RepoMetrics.TotalBranches = branchesCount

	prCount, err := countPullRequests(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting PRs: %w", err)
	}
	finalDetail.RepoMetrics.TotalPullRequests = prCount

	releasesCount, err := countReleases(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting releases: %w", err)
	}
	finalDetail.RepoMetrics.TotalReleases = releasesCount

	return nil
}

func parseScopeURL(repoURL string) (owner, repo string, err error) {
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

func fetchOrgRepos(sdk *resilientbridge.ResilientBridge, org string, maxResults int) ([]MinimalRepoInfo, error) {
	var allRepos []MinimalRepoInfo
	perPage := 100
	page := 1

	for len(allRepos) < maxResults {
		remaining := maxResults - len(allRepos)
		if remaining < perPage {
			perPage = remaining
		}

		endpoint := fmt.Sprintf("/orgs/%s/repos?per_page=%d&page=%d", org, perPage, page)
		listReq := &resilientbridge.NormalizedRequest{
			Method:   "GET",
			Endpoint: endpoint,
			Headers:  map[string]string{"Accept": "application/vnd.github+json"},
		}

		listResp, err := sdk.Request("github", listReq)
		if err != nil {
			return nil, fmt.Errorf("error fetching repos: %w", err)
		}

		if listResp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP error %d: %s", listResp.StatusCode, string(listResp.Data))
		}

		var repos []MinimalRepoInfo
		if err := json.Unmarshal(listResp.Data, &repos); err != nil {
			return nil, fmt.Errorf("error decoding repos list: %w", err)
		}

		if len(repos) == 0 {
			break
		}

		allRepos = append(allRepos, repos...)
		if len(allRepos) >= maxResults {
			break
		}
		page++
	}
	if len(allRepos) > maxResults {
		allRepos = allRepos[:maxResults]
	}
	return allRepos, nil
}

func fetchRepoDetails(sdk *resilientbridge.ResilientBridge, owner, repo string) (*RepoDetail, error) {
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

func transformToFinalRepoDetail(detail *RepoDetail) *FinalRepoDetail {
	var parent *FinalRepoDetail
	if detail.Parent != nil {
		parent = transformToFinalRepoDetail(detail.Parent)
	}
	var source *FinalRepoDetail
	if detail.Source != nil {
		source = transformToFinalRepoDetail(detail.Source)
	}

	isInOrganization := (detail.Organization != nil && detail.Organization.Type == "Organization")
	isMirror := (detail.MirrorURL != nil)
	isEmpty := (detail.Size == 0)

	sec := detail.SecurityAndAnalysis
	secretScanningEnabled := false
	secretScanningPushProtectionEnabled := false
	dependabotSecurityUpdatesEnabled := false
	secretScanningNonProviderPatternsEnabled := false
	secretScanningValidityChecksEnabled := false
	vulnerabilityAlertsEnabled := false
	if sec != nil {
		secretScanningEnabled = (sec.SecretScanning != nil && sec.SecretScanning.Status == "enabled")
		secretScanningPushProtectionEnabled = (sec.SecretScanningPushProtection != nil && sec.SecretScanningPushProtection.Status == "enabled")
		dependabotSecurityUpdatesEnabled = (sec.DependabotSecurityUpdates != nil && sec.DependabotSecurityUpdates.Status == "enabled")
		secretScanningNonProviderPatternsEnabled = (sec.SecretScanningNonProviderPatterns != nil && sec.SecretScanningNonProviderPatterns.Status == "enabled")
		secretScanningValidityChecksEnabled = (sec.SecretScanningValidityChecks != nil && sec.SecretScanningValidityChecks.Status == "enabled")
	}
	if dependabotSecurityUpdatesEnabled {
		vulnerabilityAlertsEnabled = true
	}

	var licenseJSON json.RawMessage
	if detail.License != nil {
		lj, _ := json.Marshal(detail.License)
		licenseJSON = lj
	}

	dbObj := map[string]string{"name": detail.DefaultBranch}
	dbBytes, _ := json.Marshal(dbObj)

	// Determine is_active: true if not archived and not disabled
	isActive := !(detail.Archived || detail.Disabled)

	return &FinalRepoDetail{
		GitHubRepoID:  detail.ID,
		NodeID:        detail.NodeID,
		Name:          detail.Name,
		NameWithOwner: detail.FullName,

		IsActive: isActive,

		IsPrivate:        detail.Private,
		Owner:            detail.Owner,
		Description:      detail.Description,
		CreatedAt:        detail.CreatedAt,
		UpdatedAt:        detail.UpdatedAt,
		PushedAt:         detail.PushedAt,
		HomepageURL:      detail.Homepage,
		LicenseInfo:      licenseJSON,
		Topics:           detail.Topics,
		Visibility:       detail.Visibility,
		DefaultBranchRef: dbBytes,
		Permissions:      detail.Permissions,
		Organization:     detail.Organization,
		Parent:           parent,
		Source:           source,
		Language:         detail.Language,

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
			CustomProperties:          detail.CustomProperties,
			ForkingAllowed:            detail.AllowForking,
			IsTemplate:                detail.IsTemplate,
			AllowRebaseMerge:          detail.AllowRebaseMerge,
			IsArchived:                detail.Archived,
			IsDisabled:                detail.Disabled,
		},

		SecuritySettings: SecuritySettings{
			VulnerabilityAlertsEnabled:          vulnerabilityAlertsEnabled,
			SecretScanningEnabled:               secretScanningEnabled,
			SecretScanningPushProtectionEnabled: secretScanningPushProtectionEnabled,
			DependabotSecurityUpdatesEnabled:    dependabotSecurityUpdatesEnabled,
			SecretScanningNonProviderPatterns:   secretScanningNonProviderPatternsEnabled,
			SecretScanningValidityChecksEnabled: secretScanningValidityChecksEnabled,
		},

		RepoURLs: RepoURLs{
			GitURL:   detail.GitURL,
			SSHURL:   detail.SSHURL,
			CloneURL: detail.CloneURL,
			SVNURL:   detail.SVNURL,
			HTMLURL:  detail.HTMLURL,
		},
		RepoMetrics: RepoMetrics{
			StargazersCount:    detail.StargazersCount,
			WatchersTotalCount: detail.WatchersCount,
			ForkCount:          detail.ForksCount,
			OpenIssuesCount:    detail.OpenIssuesCount,
			NetworkCount:       detail.NetworkCount,
			SubscribersCount:   detail.SubscribersCount,
			Size:               detail.Size,
		},

		IsEmpty:                       isEmpty,
		IsFork:                        detail.Fork,
		IsInOrganization:              isInOrganization,
		Locked:                        detail.Locked,
		IsMirror:                      isMirror,
		IsSecurityPolicyEnabled:       false,
		IsTemplate:                    detail.IsTemplate,
		IsUserConfigurationRepository: false,
	}
}

func countCommits(sdk *resilientbridge.ResilientBridge, owner, repoName, defaultBranch string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits?sha=%s&per_page=1", owner, repoName, defaultBranch)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countIssues(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/issues?state=all&per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countBranches(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/branches?per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countPullRequests(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=all&per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countReleases(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/releases?per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countItemsFromEndpoint(sdk *resilientbridge.ResilientBridge, endpoint string) (int, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	resp, err := sdk.Request("github", req)
	if err != nil {
		return 0, fmt.Errorf("error fetching data: %w", err)
	}

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

	if linkHeader == "" {
		if len(resp.Data) > 2 {
			var items []interface{}
			if err := json.Unmarshal(resp.Data, &items); err != nil {
				return 1, nil
			}
			return len(items), nil
		}
		return 0, nil
	}

	lastPage, err := parseLastPage(linkHeader)
	if err != nil {
		return 0, fmt.Errorf("could not parse last page: %w", err)
	}

	return lastPage, nil
}

func parseLastPage(linkHeader string) (int, error) {
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
