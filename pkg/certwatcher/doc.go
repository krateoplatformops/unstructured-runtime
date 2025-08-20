/*
Package certwatcher is a helper for reloading Certificates from disk to be used
with tls servers. It provides a helper func `GetCertificate` which can be
called from `tls.Config` and passed into your tls.Listener. For a detailed
example server view pkg/webhook/server.go.
*/
package certwatcher
