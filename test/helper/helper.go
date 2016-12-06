package helper

import (
	"fmt"
	"io/ioutil"
	"os"
)

func WithDummyCredentials(fn func(dir string)) {
	if _, err := ioutil.ReadDir("temp"); err != nil {
		if err := os.Mkdir("temp", 0755); err != nil {
			panic(err)
		}
	}

	dir, err := ioutil.TempDir("temp", "dummy-credentials")

	if err != nil {
		panic(err)
	}

	// Remove all the contents in the dir including *.pem.enc created by ReadOrUpdateCompactTLSAssets()
	// Otherwise we end up with a lot of garbage directories we failed to remove as they aren't empty in
	// config/temp, nodepool/config/temp, test/integration/temp
	defer os.RemoveAll(dir)

	for _, pairName := range []string{"ca", "apiserver", "worker", "admin", "etcd", "etcd-client"} {
		certFile := fmt.Sprintf("%s/%s.pem", dir, pairName)

		if err := ioutil.WriteFile(certFile, []byte("dummycert"), 0644); err != nil {
			panic(err)
		}

		defer os.Remove(certFile)

		keyFile := fmt.Sprintf("%s/%s-key.pem", dir, pairName)

		if err := ioutil.WriteFile(keyFile, []byte("dummykey"), 0644); err != nil {
			panic(err)
		}

		defer os.Remove(keyFile)
	}

	fn(dir)
}
