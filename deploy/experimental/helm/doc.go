// Package helm holds validation for the experimental Helm chart. The chart is
// rendered by Helm and is neither built in CI nor published; these tests run
// without a helm binary and cover the invariants a rendering tool cannot check,
// such as agreement between the chart and the application's own configuration
// keys and topology rules.
package helm
