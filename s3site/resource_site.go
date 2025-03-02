package s3site

import (
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/hashicorp/terraform/helper/schema"
)

// Part size for multipart uploads
const partSize int64 = 1024 * 1024 * 5

func resourceSite() *schema.Resource {
	return &schema.Resource{
		Create: resourceSiteCreate,
		Read:   resourceSiteRead,
		Update: resourceSiteUpdate,
		Delete: resourceSiteDelete,
		Importer: &schema.ResourceImporter{
			State: importState,
		},

		CustomizeDiff: customizeDiff,

		Schema: map[string]*schema.Schema{
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
			},
			"path": {
				Type:     schema.TypeString,
				Required: true,
			},
			"files": {
				Type:     schema.TypeMap,
				Computed: true,
			},
			"exclude": {
				Type:     schema.TypeString,
				Optional: true,
			},
		},
	}
}

func customizeDiff(diff *schema.ResourceDiff, v interface{}) error {
	path := diff.Get("path").(string)
	exclude := diff.Get("exclude").(string)

	fileMap := make(map[string]interface{})

	if localFiles, err := extractArchive(path); err != nil {
		return err
	} else {
		for _, localFile := range localFiles {
			hash, err := localFile.getMd5Checksum()

			if err != nil {
				return err
			}

			key := encodeKey(localFile.RelativePath)

			log.Printf("[DEBUG] Read file. key=%s, value=%s", key, hash)
			fileMap[key] = hash
		}
	}

	fileMap = filterMap(fileMap, exclude)

	diff.SetNew("files", fileMap)

	return nil
}

func importState(data *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	data.Set("bucket", data.Id())

	err := resourceSiteRead(data, meta)
	if err != nil {
		return nil, err
	}

	return []*schema.ResourceData{data}, nil
}

func resourceSiteCreate(data *schema.ResourceData, meta interface{}) error {
	m := meta.(*Meta)
	bucket := data.Get("bucket").(string)
	exclude := data.Get("exclude").(string)
	keyChecksumMap := data.Get("files").(map[string]interface{})

	data.SetId(bucket)

	fileMap := filterMap(keyChecksumMap, exclude)

	fileInfoMap := convertMap(fileMap)

	fileInfoMapD := decorateMap(fileInfoMap)

	if bulkUploadErr := m.S3Helper.BulkUploadS3Objects(fileInfoMapD, bucket); bulkUploadErr != nil {
		return bulkUploadErr
	}

	return nil
}

func resourceSiteRead(data *schema.ResourceData, meta interface{}) error {
	m := meta.(*Meta)
	bucket := data.Get("bucket").(string)
	exclude := data.Get("exclude").(string)

	log.Printf("[INFO] Reading bucket. bucket=%s", bucket)
	listObjectResponse, err := m.S3Helper.ListS3Objects(bucket)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				log.Printf("[DEBUG] %s. bucket=%s", s3.ErrCodeNoSuchBucket, bucket)

				data.SetId("")
				return nil
			default:
				return err
			}
		} else {
			return err
		}
	}

	data.SetId(bucket)

	fileMap := make(map[string]interface{})
	for _, bucketFile := range listObjectResponse.Contents {
		key := encodeKey(*bucketFile.Key)
		fileMap[key] = cleanS3ETag(*bucketFile.ETag)
	}

	fileMap = filterMap(fileMap, exclude)

	data.Set("files", fileMap)

	return nil
}

func convertMap(fileMap map[string]interface{}) map[string]fileInfo {
	fileInfoMap := make(map[string]fileInfo)
	for key, checksum := range fileMap {
		fileInfoMap[key] = convertKeyPair(key, checksum)
	}

	return fileInfoMap
}

func convertKeyPair(key string, value interface{}) fileInfo {
	decodedKey := decodeKey(key)
	return fileInfo{
		RelativePath: decodedKey,
		Hash:         value.(string),
		FullPath:     fmt.Sprintf("%s/site/%s", tempDir, decodedKey),
	}
}

func resourceSiteUpdate(data *schema.ResourceData, meta interface{}) error {
	m := meta.(*Meta)
	bucket := data.Get("bucket").(string)

	oldFiles, newFiles := data.GetChange("files")

	oldFileMap := oldFiles.(map[string]interface{})
	newFileMap := newFiles.(map[string]interface{})

	filesToPutMap := make(map[string]interface{})
	var filesToDelete []string

	// Need to put these files regardless if they are new or updated
	for key, value := range newFileMap {
		filesToPutMap[key] = value
	}

	// If the file doesn't exists anymore it needs to be deleted
	for key, value := range oldFileMap {
		if _, ok := newFileMap[key]; !ok {
			f := convertKeyPair(key, value)
			filesToDelete = append(filesToDelete, f.RelativePath)
		}
	}

	filesToPutFileMap := convertMap(filesToPutMap)

	filesToPutFileMapD := decorateMap(filesToPutFileMap)

	if err := m.S3Helper.BulkUploadS3Objects(filesToPutFileMapD, bucket); err != nil {
		return err
	}

	if err := m.S3Helper.DeleteObjects(bucket, filesToDelete); err != nil {
		return err
	}

	return nil
}

func resourceSiteDelete(data *schema.ResourceData, meta interface{}) error {
	m := meta.(*Meta)
	bucket := data.Get("bucket").(string)

	err := m.S3Helper.DeleteAllObjects(bucket)
	if err != nil {
		return err
	}

	return nil
}

func filterMap(fileMap map[string]interface{}, exclude string) map[string]interface{} {
	if exclude == "" {
		return fileMap
	}

	filteredFileMap := make(map[string]interface{})
	for key, value := range fileMap {
		if strings.Contains(key, exclude) {
			log.Printf("[DEBUG] Filtering out file. key=%s", key)
			continue
		}
		filteredFileMap[key] = value
	}

	return filteredFileMap
}

func decorateMap(fileInfoMap map[string]fileInfo) map[string]fileInfo {
	fileInfoMapD := make(map[string]fileInfo)
	for key, fi := range fileInfoMap {
		fileData, _ := ioutil.ReadFile(fi.FullPath)

		// DetectContentType implements https://mimesniff.spec.whatwg.org/ which doesn't support SVG
		// Use mime.TypeByExtension first then fallback to DetectContentType
		// See https://github.com/golang/go/issues/15888
		fi.ContentType = mime.TypeByExtension(path.Ext(fi.FullPath))

		// There's a situation where JS extension files have been gzipped
		// We need to handle those by sniffing the file
		if fi.ContentType == "" || strings.Contains(fi.ContentType, "javascript") {
			fi.ContentType = http.DetectContentType(fileData)
		}
				
		if strings.Contains(fi.ContentType, "gzip") && strings.Contains(filepath.Ext(fi.FullPath), ".js") {
			fi.ContentEncoding = "gzip"
			fi.ContentType = "application/javascript"			
		}

		if strings.HasSuffix(fi.FullPath, "index.html") {
			fi.CacheControl = "no-cache, no-store, must-revalidate"
			fi.Expires = "0"
		}

		fileInfoMapD[key] = fi
	}

	return fileInfoMapD
}
