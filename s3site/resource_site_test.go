package s3site

import (
	"log"
	"os"
	"testing"
)

var m map[string]fileInfo

func TestDecorateMap(t *testing.T) {
	f1, e1 := os.Stat("../test_resources/index.html")
	if e1 != nil {
		log.Fatal(e1)
	}
	fi1 := fileInfo{
		FullPath:     "../test_resources/index.html",
		RelativePath: "test",
		FileInfo:     f1,
	}

	f2, e2 := os.Stat("../test_resources/index.js")
	if e2 != nil {
		log.Fatal(e2)
	}
	fi2 := fileInfo{
		FullPath:     "../test_resources/index.js",
		RelativePath: "test",
		FileInfo:     f2,
	}

	f3, e3 := os.Stat("../test_resources/logo.svg")
	if e3 != nil {
		log.Fatal(e3)
	}
	fi3 := fileInfo{
		FullPath:     "../test_resources/logo.svg",
		RelativePath: "test",
		FileInfo:     f3,
	}

	f4, e4 := os.Stat("../test_resources/index_compressed.js")
	if e4 != nil {
		log.Fatal(e4)
	}
	fi4 := fileInfo{
		FullPath:     "../test_resources/index_compressed.js",
		RelativePath: "test",
		FileInfo:     f4,
	}

	m = make(map[string]fileInfo)
	m["1"] = fi1
	m["2"] = fi2
	m["3"] = fi3
	m["4"] = fi4

	resultMap := decorateMap(m)
	r1 := resultMap["1"]
	r2 := resultMap["2"]
	r3 := resultMap["3"]
	r4 := resultMap["4"]

	if r1.CacheControl != "no-cache, no-store, must-revalidate" {
		t.Error("Missing CacheControl on index.html")
	}

	if r1.Expires != "0" {
		t.Error("Missing Expires on index.html")
	}

	if r2.CacheControl == "no-cache, no-store, must-revalidate" {
		t.Error("Invalid CacheControl on non index.html")
	}

	if r2.Expires == "0" {
		t.Error("Invalid Expires on non index.html")
	}

	if r3.ContentType != "image/svg+xml" {
		t.Error("Invalid ContentType on logo.svg")
	}

	if r4.ContentType != "application/javascript" {
		t.Error("Invalid ContentType on main.js")
	}

	if r4.ContentEncoding != "gzip" {
		t.Error("Invalid ContentEncoding on main.js")
	}
}
