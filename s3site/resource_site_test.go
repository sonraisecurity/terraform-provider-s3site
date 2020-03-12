package s3site

import (
	"log"
	"os"
	"testing"
)

var m map[string]fileInfo

func Test_decorateMap(t *testing.T) {
	f1, e1 := os.Stat("../test_resources/index.html")
	if e1 != nil {
		log.Fatal(e1)
	}
	fi1 := fileInfo{
		FullPath:     "../test_resources/index.html",
		RelativePath: "test",
		FileInfo:     f1,
	}

	f2, e2 := os.Stat("../test_resources/my-file.js")
	if e2 != nil {
		log.Fatal(e2)
	}
	fi2 := fileInfo{
		FullPath:     "../test_resources/my-file.js",
		RelativePath: "test",
		FileInfo:     f2,
	}

	m = make(map[string]fileInfo)
	m["1"] = fi1
	m["2"] = fi2

	resultMap := decorateMap(m)
	r1 := resultMap["1"]
	r2 := resultMap["2"]

	if r1.CacheControl != "no-cache, no-store, must-revalidate" {
		t.Error("No CacheControl")
	}

	if r1.Expires != "0" {
		t.Error("No Expires")
	}

	if r2.CacheControl == "no-cache, no-store, must-revalidate" {
		t.Error("Invalid CacheControl on non index.html")
	}

	if r2.Expires == "0" {
		t.Error("Invalid on non index.html")
	}
}
