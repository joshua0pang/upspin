// Copyright 2016 The Upspin Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcs

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"upspin.io/cloud/storage"
	"upspin.io/log"
)

const defaultTestBucketName = "upspin-test-scratch"

var (
	client      storage.Storage
	testDataStr = fmt.Sprintf("This is test at %v", time.Now())
	testData    = []byte(testDataStr)
	fileName    = fmt.Sprintf("test-file-%d", time.Now().Second())

	testBucket = flag.String("test_bucket", defaultTestBucketName, "bucket name to use for testing")
	useGcloud  = flag.Bool("use_gcloud", false, "enable to run google cloud tests; requires gcloud auth login")
)

// This is more of a regression test as it uses the running cloud
// storage in prod. However, since GCP is always available, we accept
// to rely on it.
func TestPutGetAndDownload(t *testing.T) {
	link, err := client.Put(fileName, testData)
	if err != nil {
		t.Fatalf("Can't put: %v", err)
	}
	if !strings.HasPrefix(link, "https://") {
		t.Errorf("Link is not HTTPS")
	}
	retLink, err := client.Get(fileName)
	if err != nil {
		t.Fatalf("Can't get: %v", err)
	}
	if retLink != link {
		t.Errorf("Not the same link as stored: %v vs received: %v", link, retLink)
	}
	resp, err := http.Get(retLink)
	if err != nil {
		t.Errorf("Couldn't get link: %v", err)
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Can't read HTTP body: %v", err)
	}
	if string(data) != testDataStr {
		t.Errorf("Expected %q got %q", testDataStr, string(data))
	}
	// Check that Download yields the same data
	bytes, err := client.Download(fileName)
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) != testDataStr {
		t.Errorf("Expected %q got %q", testDataStr, string(bytes))
	}
}

func Put(t *testing.T, path string) {
	_, err := client.Put(path, testData)
	if err != nil {
		t.Fatal(err)
	}
}

func setupDirectoryTree(t *testing.T) {
	Put(t, "a")
	Put(t, "a/b")
	Put(t, "a/b/c")
	Put(t, "a/b/c/d1")
	Put(t, "a/b/c/d2")
	Put(t, "a/b/c/d1/e11")
	Put(t, "a/b/c/d1/e12")
	Put(t, "a/b/c/d1/e13")
	Put(t, "a/b/c/d2/e21")
	Put(t, "a/b/c/d2/e22")
	Put(t, "a/b/c/d2/e23")
}

func TestList(t *testing.T) {
	setupDirectoryTree(t)
	type testResult struct {
		prefix   string
		depth    int
		expected []string
	}

	tests := []testResult{
		testResult{
			prefix: "a/b",
			depth:  0,
			expected: []string{
				"a/b",
			},
		},
		testResult{
			prefix: "a/b",
			depth:  1,
			expected: []string{
				"a/b",
				"a/b/c",
			},
		},
		testResult{
			prefix: "a/b",
			depth:  2,
			expected: []string{
				"a/b",
				"a/b/c",
				"a/b/c/d1",
				"a/b/c/d2",
			},
		},
		testResult{
			prefix: "a/b",
			depth:  3,
			expected: []string{
				"a/b",
				"a/b/c",
				"a/b/c/d1",
				"a/b/c/d1/e11",
				"a/b/c/d1/e12",
				"a/b/c/d1/e13",
				"a/b/c/d2",
				"a/b/c/d2/e21",
				"a/b/c/d2/e22",
				"a/b/c/d2/e23",
			},
		},
		testResult{
			prefix: "a",
			depth:  4,
			expected: []string{
				"a",
				"a/b",
				"a/b/c",
				"a/b/c/d1",
				"a/b/c/d1/e11",
				"a/b/c/d1/e12",
				"a/b/c/d1/e13",
				"a/b/c/d2",
				"a/b/c/d2/e21",
				"a/b/c/d2/e22",
				"a/b/c/d2/e23",
			},
		},
	}
	for i := range tests {
		testListPrefix(t, tests[i].prefix, tests[i].depth, tests[i].expected)
	}
}

func contains(needle string, haystack []string) bool {
	for _, elem := range haystack {
		if elem == needle {
			return true
		}
	}
	return false
}

func testListPrefix(t *testing.T, prefix string, depth int, expected []string) {
	names, err := client.ListPrefix(prefix, depth)
	if err != nil {
		t.Fatalf("Error in client.List: %v", err)
	}
	t.Logf("Prefix: got: %+v", names)
	if len(names) != len(expected) {
		t.Fatalf("Expected %d results, got %d", len(expected), len(names))
	}
	for _, e := range expected {
		if !contains(e, names) {
			t.Errorf("%s %d: %q not found", prefix, depth, e)
		}
	}
}

func TestListDir(t *testing.T) {
	prefix := "a/b/c/d1/"
	names, err := client.ListDir(prefix)
	if err != nil {
		t.Fatalf("Error in client.List: %v", err)
	}
	expected := []string{prefix + "e11", prefix + "e12", prefix + "e13"}
	if len(names) != len(expected) {
		t.Fatalf("Expected 3 results, got %d", len(names))
	}
	for _, e := range expected {
		if !contains(e, names) {
			t.Errorf("Expected %q, not found", e)
		}
	}
}

func TestDelete(t *testing.T) {
	_, err := client.Put(fileName, testData)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Delete(fileName)
	if err != nil {
		t.Fatalf("Expected no errors, got %v", err)
	}
	// Test the side effect after Delete.
	_, err = client.Get(fileName)
	if err == nil {
		t.Fatal("Expected an error, but got none")
	}
}

func TestMain(m *testing.M) {
	flag.Parse()
	if !*useGcloud {
		log.Printf(`

cloud/storage/gcs: skipping test as it requires GCS access. To enable this test,
ensure you are authenticated to a GCP project that has editor permissions to a
GCS bucket named by flag -test_bucket and then set this test's flag -use_gcloud.

`)
		os.Exit(0)
	}

	// Create client that writes to test bucket.
	var err error
	client, err = storage.Dial("GCS",
		storage.WithKeyValue("gcpBucketName", *testBucket),
		storage.WithKeyValue("defaultACL", PublicRead))
	if err != nil {
		log.Fatalf("cloud/storage/gcs: couldn't set up client: %v", err)
	}

	code := m.Run()

	// Clean up.
	const verbose = true
	if err := client.(*gcsImpl).EmptyBucket(verbose); err != nil {
		log.Printf("cloud/storage/gcs: EmptyBucket failed: %v", err)
	}

	os.Exit(code)
}
