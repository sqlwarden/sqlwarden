// Package helm holds validation for the deployment charts. The chart itself is
// rendered by Helm; these tests cover the invariants that a rendering tool
// cannot check, such as agreement between the chart and the application's own
// configuration and topology rules.
package helm
