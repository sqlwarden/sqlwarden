// Package buildinfo describes the core and distribution artifacts in a composed build.
package buildinfo

// Info identifies the Community core and the distribution that embeds it.
type Info struct {
	CoreVersion         string `json:"core_version"`
	CoreCommit          string `json:"core_commit"`
	Distribution        string `json:"distribution"`
	DistributionVersion string `json:"distribution_version"`
	DistributionCommit  string `json:"distribution_commit"`
	Date                string `json:"date"`
}
