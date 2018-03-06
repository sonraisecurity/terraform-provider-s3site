package s3site

import (
	"fmt"
	"github.com/hashicorp/terraform/helper/schema"
	"log"
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

				// Elem: &schema.Resource{
				// 	Schema: map[string]*schema.Schema{
				// 		"key": {
				// 			Type:     schema.TypeString,
				// 			Required: true,
				// 		},
				// 		"hash": {
				// 			Type:     schema.TypeString,
				// 			Required: true,
				// 		},
				// 	},
				// },
			},
		},
	}
}

func customizeDiff(diff *schema.ResourceDiff, v interface{}) error {
	path := diff.Get("path").(string)

	fileMap := make(map[string]string)

	if localFiles, err := extractArchive(path); err != nil {
		return err
	} else {
		for _, localFile := range localFiles {
			hash, err := getFileMd5(localFile.FullPath)

			if err != nil {
				return err
			}

			key := encodeKey(localFile.RelativePath)

			log.Printf("[DEBUG] Read file. key=%s, value=%s", key, hash)
			fileMap[key] = hash
		}
	}

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
	bucket := data.Get("bucket").(string)
	keyChecksumMap := data.Get("files").(map[string]interface{})

	data.SetId(bucket)

	fileMap := make(map[string]fileInfo)
	for key, checksum := range keyChecksumMap {
		decodedKey := decodeKey(key)
		fileMap[key] = fileInfo{
			RelativePath: decodedKey,
			Hash:         checksum.(string),
			FullPath:     fmt.Sprintf("%s/site/%s", tempDir, decodedKey),
		}
	}

	if bulkUploadErr := bulkUploadS3Objects(fileMap, bucket, meta); bulkUploadErr != nil {
		return bulkUploadErr
	}

	return nil
}

func resourceSiteRead(data *schema.ResourceData, meta interface{}) error {
	bucket := data.Get("bucket").(string)

	log.Printf("[INFO] Reading bucket. bucket=%s", bucket)
	listObjectResponse, err := listS3Objects(bucket, meta)
	if err != nil {
		return err
	}

	data.SetId(bucket)

	fileMap := make(map[string]string)
	for _, bucketFile := range listObjectResponse.Contents {
		key := encodeKey(*bucketFile.Key)
		fileMap[key] = cleanS3ETag(*bucketFile.ETag)
	}

	data.Set("files", fileMap)

	return nil
}

func resourceSiteUpdate(data *schema.ResourceData, meta interface{}) error {
	bucket := data.Get("bucket").(string)

	log.Printf("[INFO] Clearing bucket. bucket=%s", bucket)
	if err := deleteAllObjects(bucket, meta); err != nil {
		return err
	}

	if err := resourceSiteCreate(data, meta); err != nil {
		return err
	}

	return nil
}

func resourceSiteDelete(data *schema.ResourceData, meta interface{}) error {
	bucket := data.Get("bucket").(string)

	err := deleteAllObjects(bucket, meta)
	if err != nil {
		return err
	}

	return nil
}
