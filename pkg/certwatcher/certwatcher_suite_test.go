package certwatcher_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	certPath = "testdata/tls.crt"
	keyPath  = "testdata/tls.key"
)

func TestSource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CertWatcher Suite")
}

var _ = BeforeSuite(func() {
})

var _ = AfterSuite(func() {
	for _, file := range []string{certPath, keyPath, certPath + ".new", keyPath + ".new", certPath + ".old", keyPath + ".old"} {
		_ = os.Remove(file)
	}
})
