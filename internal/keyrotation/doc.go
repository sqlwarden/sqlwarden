// Package keyrotation re-encrypts application-encrypted data with the
// keyring's primary key. One service backs both the instance-admin HTTP
// endpoint and the operator's rotate-keys command; the stages themselves live
// behind ports owned by credentials, settings, and files.
package keyrotation
