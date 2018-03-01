package s3site

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/mholt/archiver"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

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
			// artifact_url will be moved to its own data provider
			"artifact_url": {
				Type:     schema.TypeString,
				Required: true,
			},
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
			},
			"files": {
				Type:     schema.TypeMap,
				Computed: true,
			},
		},
	}
}

func importState(data *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	data.Set("artifact_url", "temp")
	data.Set("bucket", data.Id())

	err := resourceSiteRead(data, meta)
	if err != nil {
		return nil, err
	}

	return []*schema.ResourceData{data}, nil
}

func customizeDiff(diff *schema.ResourceDiff, v interface{}) error {
	artifactUrl := diff.Get("artifact_url").(string)

	localFiles, err := extractArchive(artifactUrl)
	if err != nil {
		return err
	}

	fileMap := make(map[string]string)

	for _, localFile := range localFiles {
		hash, err := getFileMd5(localFile.FullPath)

		if err != nil {
			return err
		}

		fileMap[localFile.RelativePath] = hash
	}

	diff.SetNew("files", fileMap)

	return nil
}

func resourceSiteCreate(d *schema.ResourceData, meta interface{}) error {
	artifactUrl := d.Get("artifact_url").(string)
	bucket := d.Get("bucket").(string)

	d.SetId(bucket)

	localFiles, err := extractArchive(artifactUrl)
	if err != nil {
		return err
	}

	bulkUploadErr := bulkUploadS3Objects(localFiles, bucket, meta)
	if bulkUploadErr != nil {
		return bulkUploadErr
	}

	fileMap := make(map[string]string)
	for _, localFile := range localFiles {
		if err != nil {
			return err
		}

		hash, err := getFileMd5(localFile.FullPath)
		if err != nil {
			return err
		}

		fileMap[localFile.RelativePath] = cleanS3ETag(hash)
	}

	d.Set("files", fileMap)

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
		log.Printf("[DEBUG] key=%s, etag=%s", *bucketFile.Key, *bucketFile.ETag)
		fileMap[*bucketFile.Key] = cleanS3ETag(*bucketFile.ETag)
	}

	data.Set("files", fileMap)

	return nil
}

func resourceSiteUpdate(data *schema.ResourceData, meta interface{}) error {
	bucket := data.Get("bucket").(string)

	err := deleteAllObjects(bucket, meta)
	if err != nil {
		return err
	}

	err = resourceSiteCreate(data, meta)
	if err != nil {
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

func listS3Objects(bucket string, meta interface{}) (*s3.ListObjectsV2Output, error) {
	sess := meta.(*session.Session)
	s3conn := s3.New(sess)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	return s3conn.ListObjectsV2(input)
}

func deleteAllObjects(bucket string, meta interface{}) error {
	listObjectResponse, err := listS3Objects(bucket, meta)

	if err != nil {
		return err
	}

	var keys []string
	for _, object := range listObjectResponse.Contents {
		keys = append(keys, *object.Key)
	}

	return bulkDeleteS3Objects(bucket, keys, meta)
}

func bulkUploadS3Objects(files []fileInfo, bucket string, meta interface{}) error {
	sess := meta.(*session.Session)

	uploader := s3manager.NewUploader(sess)

	for _, fileInfo := range files {
		file, openErr := os.Open(fileInfo.FullPath)
		if openErr != nil {
			return openErr
		}

		reader := bufio.NewReader(file)

		uploadInput := &s3manager.UploadInput{
			Bucket: &bucket,
			Key:    &fileInfo.RelativePath,
			Body:   reader,
		}

		_, uploaderErr := uploader.Upload(uploadInput)
		if uploaderErr != nil {
			return uploaderErr
		}
	}

	return nil
}

func bulkDeleteS3Objects(bucket string, keys []string, meta interface{}) error {
	sess := meta.(*session.Session)

	batcher := s3manager.NewBatchDelete(sess)

	batchDeleteObjects := []s3manager.BatchDeleteObject{}

	for _, key := range keys {
		batchDeleteObject := s3manager.BatchDeleteObject{
			Object: &s3.DeleteObjectInput{
				Key:    &key,
				Bucket: &bucket,
			},
		}

		batchDeleteObjects = append(batchDeleteObjects, batchDeleteObject)
	}

	err := batcher.Delete(aws.BackgroundContext(), &s3manager.DeleteObjectsIterator{
		Objects: batchDeleteObjects,
	})

	if err != nil {
		return err
	}

	return nil
}

func getFileMd5(path string) (string, error) {
	content, err := ioutil.ReadFile(path)

	if err != nil {
		return "", err
	}

	hash := md5.Sum(content)
	return fmt.Sprintf("%x", hash), nil
}

func cleanS3ETag(eTag string) string {
	return strings.Trim(eTag, "\\\"")
}

func extractArchive(archive string) ([]fileInfo, error) {
	tempFolder := "/tmp/s3site/"
	err := archiver.Zip.Open(archive, tempFolder)
	if err != nil {
		return nil, err
	}

	var fileList []fileInfo
	err = filepath.Walk(tempFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		fileList = append(fileList, fileInfo{
			FullPath:     path,
			RelativePath: strings.TrimPrefix(path, tempFolder),
			FileInfo:     info,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return fileList, nil
}

type fileInfo struct {
	FullPath     string
	RelativePath string
	FileInfo     os.FileInfo
}
